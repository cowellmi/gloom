{
  description = "Gloom TinyGo Feather M0 dev environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };

        bossac-adafruit = pkgs.stdenv.mkDerivation {
          pname = "bossac-adafruit";
          version = "1.8.0-48-gb176eee";

          src = pkgs.fetchFromGitHub {
            owner = "adafruit";
            repo = "BOSSA";
            rev = "b176eee";
            hash = "sha256-WztLY+ScOk5UX1E2PH6uJb+7o2iVyAjzHyrzoWKpakc=";
          };

          buildInputs = [ pkgs.readline ];

          postPatch = ''
            substituteInPlace Makefile \
              --replace "-arch x86_64" ""
          '';

          env.NIX_CFLAGS_COMPILE = toString (
            [ "-Wno-error=deprecated-declarations" ]
            ++ pkgs.lib.optionals pkgs.stdenv.cc.isClang [
              "-Wno-error=vla-cxx-extension"
            ]
          );

          makeFlags = [ "bin/bossac" ];

          installPhase = ''
            mkdir -p $out/bin
            cp bin/bossac $out/bin/
          '';

          meta = {
            description = "Adafruit fork of BOSSA flash programmer";
            homepage = "https://github.com/adafruit/BOSSA";
            license = pkgs.lib.licenses.bsd3;
          };
        };
      in
      {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            gnumake
            go
            tinygo
            tio
            bossac-adafruit
          ];
        };
      });
}
