mac_frameworks = -F${CURDIR}/MacOSX_SDK/System/Library/Frameworks/
mac_libraries = -L${CURDIR}/MacOSX_SDK/usr/lib
ld_flags = -ldflags='-s -w'

## Principal operations

prepare:
	go get .

build:
	go build ${ld_flags} -o out/tftpcpd

debug:
	dlv debug

test:
	go test -tags test -- -T

clean:
	rm -f out/tftpcpd*



## Cross compilation
# Use 'build' to build for your native platform
# Only worry about this if you want to distribute binaries targeting multiple operating systems

release: out/tftpcpd.mac.amd64 out/tftpcpd.mac.arm64 out/tftpcpd.exe out/tftpcpd.linux.amd64 out/tftpcpd.linux.arm64

out/tftpcpd.exe:
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC="zig cc -target x86_64-windows" CXX="zig c++ -target x86_64-windows" go build ${ld_flags} -tags "windows amd64" -o out/tftpcpd.exe

out/tftpcpd.linux.amd64:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC="zig cc -target x86_64-linux" CXX="zig c++ -target x86_64-linux" go build ${ld_flags} -tags "linux amd64" -o out/tftpcpd.linux.amd64

out/tftpcpd.linux.arm64:
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC="zig cc -target aarch64-linux" CXX="zig c++ -target aarch64-linux" go build ${ld_flags} -tags "linux arm64" -o out/tftpcpd.linux.arm64

out/tftpcpd.mac.amd64:
	[ ! -d "./MacOSX_SDK" ] && echo "UNABLE TO FIND REQUIRED MAC OS SDK DIRECTORY" 1>&2 && exit 1 || true
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 CC="zig cc -target x86_64-macos ${mac_frameworks} ${mac_libraries}" CXX="zig c++ -target x86_64-macos ${mac_frameworks} ${mac_libraries}" go build ${ld_flags} -tags "darwin amd64" -o out/tftpcpd.mac.amd64

out/tftpcpd.mac.arm64:
	[ ! -d "./MacOSX_SDK" ] && echo "UNABLE TO FIND REQUIRED MAC OS SDK DIRECTORY" 1>&2 && exit 1 || true
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 CC="zig cc -target aarch64-macos ${mac_frameworks} ${mac_libraries}" CXX="zig c++ -target aarch64-macos ${mac_frameworks} ${mac_libraries}" go build ${ld_flags} -tags "darwin arm64" -o out/tftpcpd.mac.arm64
