{ self }:
{ pkgs, ... }:
{
  name = "alias-watch-cli";

  nodes.machine =
    { ... }:
    {
      imports = [ self.nixosModules.default ];

      users.users.alice = {
        isNormalUser = true;
        extraGroups = [ "alias-watch" ];
      };

      environment.etc."alias-watch/env".text = ''
        IMAP_PASSWORD=test-imap-password
      '';

      services.alias-watch = {
        enable = true;
        settings = {
          imap = {
            server = "127.0.0.1";
            username = "alice";
            tls = false;
          };
          notify.ntfy_url = "http://127.0.0.1:18081/topic";
        };
        environmentFile = "/etc/alias-watch/env";
      };
    };

  testScript = ''
    start_all()

    machine.wait_for_unit("multi-user.target")

    machine.succeed("""
      set -euo pipefail
      command -v alias-watch
      su -s /bin/sh -c 'alias-watch validate' alice
      test -f /var/lib/alias-watch/data.db
      test "$(stat -c '%a' /var/lib/alias-watch/data.db)" = "660"
      test "$(stat -c '%G' /var/lib/alias-watch/data.db)" = "alias-watch"
    """)
  '';
}
