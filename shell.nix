{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  buildInputs = with pkgs; [
    go
    zenity
    gcc
    pkg-config
    xorg.libX11.dev
    xorg.xorgproto
    libGL
    xorg.libXcursor
    xorg.libXinerama
    xorg.libXrandr
    xorg.libXi
    xorg.libXxf86vm
    xorg.libX11.dev
    xorg.libXext.dev
    pkgsCross.mingwW64.buildPackages.gcc
    pkgsCross.mingwW64.buildPackages.binutils
  ];

  shellHook = ''
    export PKG_CONFIG_PATH="${pkgs.xorg.libX11.dev}/lib/pkgconfig:${pkgs.xorg.xorgproto}/share/pkgconfig:$PKG_CONFIG_PATH"
    export PATH=${pkgs.pkgsCross.mingwW64.buildPackages.gcc}/bin:$PATH
    
    echo "Development environment loaded"
  '';
}
