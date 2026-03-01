{ self }:
{ pkgs, lib, ... }:
{
  name = "alias-watch-full-stack";

  nodes.machine =
    { pkgs, ... }:
    {
      imports = [ self.nixosModules.default ];

      users.users.alice = {
        isNormalUser = true;
        initialPassword = "foobar";
        extraGroups = [ "alias-watch" ];
      };

      security.sudo.extraRules = [
        {
          users = [ "alice" ];
          commands = [
            {
              command = "ALL";
              options = [ "NOPASSWD" ];
            }
          ];
        }
      ];

      services.dovecot2 = {
        enable = true;
        enableImap = true;
        mailLocation = "maildir:~/mail";
        protocols = [ "imap" ];
      };

      systemd.services.mock-ntfy = {
        description = "Mock ntfy endpoint";
        wantedBy = [ "multi-user.target" ];
        after = [ "network.target" ];
        serviceConfig = {
          Type = "simple";
          ExecStart = "${pkgs.python3}/bin/python ${pkgs.writeText "mock-ntfy.py" ''
            from http.server import BaseHTTPRequestHandler, HTTPServer
            import os

            LOG_FILE = "/var/lib/mock-ntfy/requests.log"
            os.makedirs("/var/lib/mock-ntfy", exist_ok=True)
            open(LOG_FILE, "a", encoding="utf-8").close()

            class Handler(BaseHTTPRequestHandler):
                def do_POST(self):
                    length = int(self.headers.get("Content-Length", "0"))
                    body = self.rfile.read(length).decode("utf-8", "replace")
                    with open(LOG_FILE, "a", encoding="utf-8") as f:
                        f.write(body + "\n---\n")
                    self.send_response(200)
                    self.end_headers()

                def log_message(self, fmt, *args):
                    pass

            HTTPServer(("127.0.0.1", 18081), Handler).serve_forever()
          ''}";
          Restart = "always";
          RestartSec = "1s";
        };
      };

      environment.etc."alias-watch/config.yaml".text = ''
        imap:
          server: "127.0.0.1"
          port: 143
          username: "alice"
          folder: "INBOX"
          tls: false

        db:
          path: "/var/lib/alias-watch/data.db"

        notify:
          ntfy_url: "http://127.0.0.1:18081/topic"
      '';

      environment.etc."alias-watch/env".text = ''
        IMAP_PASSWORD=foobar
      '';

      services.alias-watch = {
        enable = true;
        settings = {
          imap = {
            server = "127.0.0.1";
            port = 143;
            username = "alice";
            folder = "INBOX";
            tls = false;
          };
          db.path = "/var/lib/alias-watch/data.db";
          notify.ntfy_url = "http://127.0.0.1:18081/topic";
        };
        environmentFile = "/etc/alias-watch/env";
      };

      environment.systemPackages = with pkgs; [
        sqlite
      ];

      networking.firewall.allowedTCPPorts = [
        143
        18081
      ];
    };

  testScript = ''
    start_all()

    machine.wait_for_unit("network-online.target")
    machine.wait_until_succeeds("systemctl is-active --quiet dovecot2.service || systemctl is-active --quiet dovecot.service")
    machine.wait_for_unit("mock-ntfy.service")
    machine.wait_for_open_port(143)
    machine.wait_for_open_port(18081)

    machine.succeed("""
      # Use printf to avoid heredoc indentation adding leading whitespace to headers.
      printf '%s\n' \
        'From: Known Sender <known.sender@example.net>' \
        'To: alias1@anonaddy.com' \
        'Delivered-To: alias1@anonaddy.com' \
        'Subject: Historical email' \
        'Message-Id: <history-1@example.net>' \
        "" \
        'historical body' \
        | ${pkgs.dovecot}/bin/doveadm save -u alice -m INBOX
    """)

    machine.succeed("""
      set -euo pipefail
      command -v alias-watch
      sqlite3 /var/lib/alias-watch/data.db \
        "INSERT INTO aliases (email, addy_id, active, title, synced_at) VALUES ('alias1@anonaddy.com', 'seed-1', 1, 'Seeded alias', datetime('now'));"
      su -s /bin/sh -c 'alias-watch learn' alice
    """)

    machine.succeed("""
      set -euo pipefail
      sqlite3 /var/lib/alias-watch/data.db \
        "SELECT email FROM aliases WHERE email='alias1@anonaddy.com';" \
        | grep -Fx 'alias1@anonaddy.com'
      sqlite3 /var/lib/alias-watch/data.db \
        "SELECT COUNT(*) FROM known_senders;" \
        | grep -E '^[0-9]+$'
    """)

    machine.systemctl("restart alias-watch.service")
    machine.wait_for_unit("alias-watch.service")
    machine.wait_until_succeeds("""
      journalctl -u alias-watch.service -b --no-pager | grep -F "Starting IDLE monitor"
    """)

    machine.succeed("""
      # Use printf to avoid heredoc indentation adding leading whitespace to headers.
      printf '%s\n' \
        'From: New Sender <new.sender@example.net>' \
        'To: alias1@anonaddy.com' \
        'Delivered-To: alias1@anonaddy.com' \
        'Subject: Monitor test message' \
        'Message-Id: <monitor-1@example.net>' \
        "" \
        'monitor test body' \
        | ${pkgs.dovecot}/bin/doveadm save -u alice -m INBOX
    """)

    machine.wait_until_succeeds("""
      grep -F "From:    new.sender@example.net" /var/lib/mock-ntfy/requests.log
    """)
  '';
}
