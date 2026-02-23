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
        lib = pkgs.lib;

        tinygo-fork = pkgs.tinygo.overrideAttrs (old: rec {
          version = "0.40.1-feather-m0-uart0";
          src = pkgs.fetchFromGitHub {
            owner = "cowellmi";
            repo = "tinygo";
            rev = "0173ee3814ee915533931a2f61b702f20c69ee43";
            hash = "sha256-rQSM1V1loGLTLhYtGQHEoi8T+qPd2rbVSrIaYRZiaQQ=";
            fetchSubmodules = true;
            postFetch = ''
              rm -r $out/lib/cmsis-svd/data/{SiliconLabs,Freescale}
            '';
          };
          goModules = old.goModules.overrideAttrs {
            inherit src;
          };
        });

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
            git
            gnumake
            go
            jq
            tinygo-fork
            tio
            bossac-adafruit
          ];

          shellHook = ''
            VSCODE_SETTINGS=".vscode/settings.json"
            if [ -f "$VSCODE_SETTINGS" ]; then
              tmp=$(jq '
                .["go.toolsEnvVars"] |= ({"GOTOOLCHAIN":"local","GOSUMDB":"off"} + (. // {}))
              ' "$VSCODE_SETTINGS") && echo "$tmp" > "$VSCODE_SETTINGS"
            else
              mkdir -p .vscode
              cat > "$VSCODE_SETTINGS" <<'EOF'
{
  "go.toolsEnvVars": {
    "GOTOOLCHAIN": "local",
    "GOSUMDB": "off"
  }
}
EOF
            fi
          '';
        };
      });
}
