{

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-compat = {
      url = "github:edolstra/flake-compat";
      flake = false;
    };
    flake-utils.url = "github:numtide/flake-utils";
    nix-bundle = {url = "github:matthewbauer/nix-bundle"; inputs.nixpkgs.follows = "nixpkgs"; };
  };
  outputs = { self, nixpkgs, flake-utils, nix-bundle, ... }:
    let

      version = builtins.replaceStrings [ "\n" ] [ "" ]
        (builtins.readFile ./.version + versionSuffix);
      versionSuffix = if officialRelease then
        ""
      else
        "pre${
          nixpkgs.lib.substring 0 8 (self.lastModifiedDate or self.lastModified)
        }_${self.shortRev or "dirty"}";

      officialRelease = false;

    in flake-utils.lib.eachDefaultSystem (system:
      let pkgs = nixpkgs.legacyPackages.${system};
      in rec {

        packages = flake-utils.lib.flattenTree rec {
          ifconfigio = pkgs.buildGoModule {
            name = "ifconfig.io-${version}";

            src = self;
            vendorSha256 = "sha256-KUgKselGjYI0I1zT/LB48pksswNXLbrgBM6LtYPeT/Q=";

            postInstall = ''
              mkdir -p $out/usr/lib/ifconfig.io/
              cp -r ./templates $out/usr/lib/ifconfig.io
            '';
          };

          docker-image = pkgs.dockerTools.buildLayeredImage {
            name = "ifconfig.io";
            tag = version;
            created = "now";
            contents = [ ifconfigio pkgs.busybox ];
            config = {
              Cmd = "/bin/ifconfig.io";
              WorkingDir = "/usr/lib/ifconfig.io";
              ExposedPorts = { "8080" = { }; };
              Env = [ "HOSTNAME=ifconfig.io" "TLS=0" "TLSCERT=" "TLSKEY=" ];
            };
          };

        };
        defaultPackage = packages.ifconfigio;
        apps.ifconfigio =  { type = "app"; program =  "${packages.ifconfigio}/bin/ifconfig.io"; };
        defaultApp = apps.ifconfigio;

        defaultBundler = nix-bundle.bundlers.nix-bundle;

      });
}
