# Navmesh Viewer

### Dependencies

Debian:
```
sudo apt-get install libx11-dev xorg-dev libgl1-mesa-dev libxcursor-dev libxinerama-dev libxrandr-dev libxi-dev libxxf86vm-dev gcc-mingw-w64 libglu1-mesa-dev mingw-w64 libglfw3-dev
```

RHEL:
```
sudo dnf install xorg-x11-server-devel libX11-devel mesa-libGL-devel libXcursor-devel libXinerama-devel libXrandr-devel libXi-devel libXxf86vm-devel mingw64-gcc mingw64-gcc-c++ mingw64-winpthreads-static mingw64-winpthreads mingw64-headers mingw64-crt
```

Arch:
```
sudo pacman -S xorg-server-devel libx11 libxcursor libxinerama libxrandr libxi libxxf86vm mingw-w64-gcc base-devel mesa libxinerama glfw-x11
```

### Build for Windows

```
CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 go build
```