{ config, lib, pkgs, ... }:

let
  cfg = config.services.alias-watch;
  yamlFormat = pkgs.formats.yaml { };
  generatedConfig = yamlFormat.generate "alias-watch-config.yaml" cfg.settings;
in
{
  options.services.alias-watch = {
    enable = lib.mkEnableOption "alias-watch daemon";

    package = lib.mkOption {
      type = lib.types.package;
      example = lib.literalExpression "inputs.alias-watch.packages.${pkgs.system}.alias-watch";
      description = "Package that provides the alias-watch binary.";
    };

    settings = lib.mkOption {
      type = yamlFormat.type;
      default = { };
      example = lib.literalExpression ''
        {
          imap = {
            server = "imap.example.com";
            port = 993;
            username = "you@example.com";
            folder = "INBOX";
            tls = true;
          };
          db.path = "/var/lib/alias-watch/data.db";
          # Optional:
          # notify = {
          #   ntfy_url = "https://ntfy.sh/your-topic";
          #   ntfy_token = "your-token";
          # };
        }
      '';
      description = "YAML settings for alias-watch.";
    };

    user = lib.mkOption {
      type = lib.types.str;
      default = "alias-watch";
      description = "User account under which the service runs.";
    };

    group = lib.mkOption {
      type = lib.types.str;
      default = "alias-watch";
      description = "Group under which the service runs.";
    };

    stateDir = lib.mkOption {
      type = lib.types.str;
      default = "/var/lib/alias-watch";
      description = "State directory for the service process.";
    };

    extraArgs = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      description = "Additional command-line arguments for alias-watch monitor.";
    };

    environmentFile = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "/run/secrets/alias-watch.env";
      description = "Optional systemd EnvironmentFile for the service (e.g. IMAP_SERVER, IMAP_USERNAME, IMAP_PASSWORD, NTFY_URL, NTFY_TOKEN).";
    };
  };

  config = lib.mkIf cfg.enable {
    services.alias-watch.settings.db.path = lib.mkDefault "${cfg.stateDir}/data.db";

    assertions = [
      {
        assertion = cfg.settings != { };
        message = "services.alias-watch.settings must be set.";
      }
    ];

    users.groups = lib.mkIf (cfg.group == "alias-watch") {
      alias-watch = { };
    };

    users.users = lib.mkIf (cfg.user == "alias-watch") {
      alias-watch = {
        isSystemUser = true;
        group = cfg.group;
        home = cfg.stateDir;
        createHome = true;
      };
    };

    systemd.tmpfiles.rules = [
      "d ${cfg.stateDir} 2770 ${cfg.user} ${cfg.group} - -"
      "f ${cfg.settings.db.path} 0660 ${cfg.user} ${cfg.group} - -"
    ];

    environment.systemPackages = [
      (lib.hiPrio (pkgs.writeShellScriptBin "alias-watch" ''
        umask 0007
        exec ${config.security.wrapperDir}/sudo -u ${lib.escapeShellArg cfg.user} -- \
          ${pkgs.runtimeShell} -c '
            if [ -n "$1" ]; then
              set -a
              . "$1"
              set +a
            fi
            shift
            exec "$@"
          ' alias-watch-wrapper \
          ${lib.escapeShellArg (if cfg.environmentFile == null then "" else cfg.environmentFile)} \
          ${lib.getExe cfg.package} --config ${lib.escapeShellArg generatedConfig} "$@"
      ''))
    ];

    systemd.services.alias-watch = {
      description = "Alias Watch Service";
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      wantedBy = [ "multi-user.target" ];

      serviceConfig =
        {
          Type = "simple";
          User = cfg.user;
          Group = cfg.group;
          WorkingDirectory = cfg.stateDir;
          ExecStart = lib.concatStringsSep " " (
            [
              "${lib.getExe cfg.package}"
              "monitor"
              "--config"
              (lib.escapeShellArg generatedConfig)
            ]
            ++ (map lib.escapeShellArg cfg.extraArgs)
          );
          Restart = "on-failure";
          RestartSec = "5s";
          UMask = "0007";
        }
        // lib.optionalAttrs (cfg.environmentFile != null) {
          EnvironmentFile = cfg.environmentFile;
        };
    };
  };
}
