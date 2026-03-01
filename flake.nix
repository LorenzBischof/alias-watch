{
  description = "Alias monitoring tool with NixOS module";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs { inherit system; };
      module = import ./nix/module.nix;
      makeTest = import (nixpkgs + "/nixos/tests/make-test-python.nix");
      mkPackage =
        pkgs:
        pkgs.buildGoModule {
          pname = "alias-watch";
          version = "0.1.0";
          src = ./.;
          subPackages = [ "cmd/alias-watch" ];
          vendorHash = "sha256-8tNJskvNLAEHJYwKg7FrCacZZKYwY34Zwige9albD7c=";
          meta.mainProgram = "alias-watch";
        };
    in
    {
      packages.${system} = {
        alias-watch = mkPackage pkgs;
        default = self.packages.${system}.alias-watch;
      };

      checks.${system} = {
        cli = (makeTest (import ./nix/tests/cli.nix { inherit self; })) {
          inherit system pkgs;
        };

        full-stack = (makeTest (import ./nix/tests/full-stack.nix { inherit self; })) {
          inherit system pkgs;
        };
      };

      nixosModules = {
        alias-watch =
          {
            lib,
            pkgs,
            ...
          }:
          {
            imports = [ module ];
            services.alias-watch.package =
              lib.mkDefault
                self.packages.${pkgs.stdenv.hostPlatform.system}.alias-watch;
          };
        default = self.nixosModules.alias-watch;
      };
    };
}
