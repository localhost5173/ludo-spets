{
  description = "Dev environment for Ludo game in Go with OpenGL";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs, ... }:
    let
      system = "x86_64-linux";
      pkgs = nixpkgs.legacyPackages.${system};
    in {
      devShells.${system}.default = pkgs.mkShell {
        buildInputs = [
          pkgs.go
          pkgs.pkg-config
          pkgs.mesa
          pkgs.libGL
          pkgs.glfw
          pkgs.xorg.libX11
          pkgs.xorg.libXrandr
          pkgs.xorg.libXcursor
          pkgs.xorg.libXinerama
          pkgs.xorg.libXi
          pkgs.xorg.libXxf86vm
          pkgs.openal
          pkgs.chromium
        ];

        shellHook = ''
          # Use the main mesa output for includes
          export CGO_CFLAGS="-I${pkgs.mesa}/include"
          export CGO_LDFLAGS="-L${pkgs.mesa}/lib -lGL"
          export PKG_CONFIG_PATH="${pkgs.glfw}/lib/pkgconfig"
          export LD_LIBRARY_PATH="${pkgs.mesa}/lib:${pkgs.glfw}/lib"
          
          # Allow Nix store paths in CGO flags
          export CGO_CFLAGS_ALLOW='-I/nix/store/.*'
          export CGO_LDFLAGS_ALLOW='-L/nix/store/.*'
        '';
      };
    };
}
