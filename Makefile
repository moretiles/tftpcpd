mac_frameworks = -F${CURDIR}/MacOSX_SDK/System/Library/Frameworks/
mac_libraries = -L${CURDIR}/MacOSX_SDK/usr/lib
ld_flags = -ldflags='-s -w'

## Principal operations

.PHONY: all prepare build debug test tags clean

all: prepare build

prepare:
	go get ./internal
	go get ./cmd/tftpcpc
	go get ./cmd/tftpcpd

build:
	go build -o dist/tftpcpc ${ld_flags} ./cmd/tftpcpc
	go build -o dist/tftpcpd ${ld_flags} ./cmd/tftpcpd

debug:
	dlv debug ./cmd/tftpcpd

test:
	go test -tags test ./internal -- -testing

tags:
	ctags -R internal cmd

clean:
	rm -f dist/tftpcpc*
	rm -f dist/tftpcpd*



## Cross compilation
# Use 'build' to build for your native platform
# Only worry about this if you want to distribute binaries targeting multiple operating systems

release: dist/tftpcpd.amd64.mac dist/tftpcpd.arm64.mac dist/tftpcpd.amd64.exe dist/tftpcpd.arm64.exe dist/tftpcpd.amd64.linux dist/tftpcpd.arm64.linux dist/tftpcpc.amd64.mac dist/tftpcpc.arm64.mac dist/tftpcpc.amd64.exe dist/tftpcpc.arm64.exe dist/tftpcpc.amd64.linux dist/tftpcpc.arm64.linux

dist/tftpcpd.amd64.exe:
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC="zig cc -target x86_64-windows" CXX="zig c++ -target x86_64-windows" go build -o dist/tftpcpd.amd64.exe ${ld_flags} -tags "windows amd64" ./cmd/tftpcpd

dist/tftpcpc.amd64.exe:
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC="zig cc -target x86_64-windows" CXX="zig c++ -target x86_64-windows" go build -o dist/tftpcpc.amd64.exe ${ld_flags} -tags "windows amd64" ./cmd/tftpcpc

dist/tftpcpd.arm64.exe:
	CGO_ENABLED=1 GOOS=windows GOARCH=arm64 CC="zig cc -target aarch64-windows" CXX="zig c++ -target aarch64-windows" go build -o dist/tftpcpd.arm64.exe ${ld_flags} -tags "windows arm64" ./cmd/tftpcpd

dist/tftpcpc.arm64.exe:
	CGO_ENABLED=1 GOOS=windows GOARCH=arm64 CC="zig cc -target aarch64-windows" CXX="zig c++ -target aarch64-windows" go build -o dist/tftpcpc.arm64.exe ${ld_flags} -tags "windows arm64" ./cmd/tftpcpc

dist/tftpcpd.amd64.linux:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC="zig cc -target x86_64-linux" CXX="zig c++ -target x86_64-linux" go build -o dist/tftpcpd.amd64.linux ${ld_flags} -tags "linux amd64" ./cmd/tftpcpd

dist/tftpcpc.amd64.linux:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC="zig cc -target x86_64-linux" CXX="zig c++ -target x86_64-linux" go build -o dist/tftpcpc.amd64.linux ${ld_flags} -tags "linux amd64" ./cmd/tftpcpc

dist/tftpcpd.arm64.linux:
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC="zig cc -target aarch64-linux" CXX="zig c++ -target aarch64-linux" go build -o dist/tftpcpd.arm64.linux ${ld_flags} -tags "linux arm64" ./cmd/tftpcpd

dist/tftpcpc.arm64.linux:
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC="zig cc -target aarch64-linux" CXX="zig c++ -target aarch64-linux" go build -o dist/tftpcpc.arm64.linux ${ld_flags} -tags "linux arm64" ./cmd/tftpcpc

dist/tftpcpd.amd64.mac:
	[ ! -d "./MacOSX_SDK" ] && echo "UNABLE TO FIND REQUIRED MAC OS SDK DIRECTORY" 1>&2 && exit 1 || true
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 CC="zig cc -target x86_64-macos ${mac_frameworks} ${mac_libraries}" CXX="zig c++ -target x86_64-macos ${mac_frameworks} ${mac_libraries}" go build -o dist/tftpcpd.amd64.mac ${ld_flags} -tags "darwin amd64" ./cmd/tftpcpd

dist/tftpcpc.amd64.mac:
	[ ! -d "./MacOSX_SDK" ] && echo "UNABLE TO FIND REQUIRED MAC OS SDK DIRECTORY" 1>&2 && exit 1 || true
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 CC="zig cc -target x86_64-macos ${mac_frameworks} ${mac_libraries}" CXX="zig c++ -target x86_64-macos ${mac_frameworks} ${mac_libraries}" go build -o dist/tftpcpc.amd64.mac ${ld_flags} -tags "darwin amd64" ./cmd/tftpcpc

dist/tftpcpd.arm64.mac:
	[ ! -d "./MacOSX_SDK" ] && echo "UNABLE TO FIND REQUIRED MAC OS SDK DIRECTORY" 1>&2 && exit 1 || true
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 CC="zig cc -target aarch64-macos ${mac_frameworks} ${mac_libraries}" CXX="zig c++ -target aarch64-macos ${mac_frameworks} ${mac_libraries}" go build -o dist/tftpcpd.arm64.mac ${ld_flags} -tags "darwin arm64" ./cmd/tftpcpd

dist/tftpcpc.arm64.mac:
	[ ! -d "./MacOSX_SDK" ] && echo "UNABLE TO FIND REQUIRED MAC OS SDK DIRECTORY" 1>&2 && exit 1 || true
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 CC="zig cc -target aarch64-macos ${mac_frameworks} ${mac_libraries}" CXX="zig c++ -target aarch64-macos ${mac_frameworks} ${mac_libraries}" go build -o dist/tftpcpc.arm64.mac ${ld_flags} -tags "darwin arm64" ./cmd/tftpcpc
