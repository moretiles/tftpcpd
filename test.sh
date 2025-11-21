#!/usr/bin/bash

# Using a bash script to test the executables themselves, rather than component functions

addr="127.0.0.1"
port="8173"
host="${addr}:${port}"

killall tftpcpd

sleep 3

rm -f ./tests/root/*.bin
rm -f ./tests/root/*.*[0-9]
rm -f ./tests/tmp/*.bin
rm -f ./tests/tmp/*.*[0-9]
rm -f ./tests/db/*.db
rm -f ./tests/db/*.db*

cp ./tests/data/foo1.bin.bak ./tests/data/foo1.bin
cp ./tests/data/foo2.bin.bak ./tests/data/foo2.bin
cp ./tests/data/foo3.bin.bak ./tests/data/foo3.bin
cp ./tests/data/foo4.bin.bak ./tests/data/foo4.bin

make build

sleep 3

./dist/tftpcpd -directory ./tests/root/ -sqlite3-db ./tests/db/tftpcpd.db "${host}" &

sleep 3

# basic upload and download with curl

curl -T ./tests/data/foo1.bin "tftp://${host}"
curl --output ./tests/tmp/foo1.bin "tftp://${host}/foo1.bin"
if ! diff -q ./tests/data/foo1.bin ./tests/tmp/foo1.bin; then
    killall tftpcpd
    sleep 3
    echo "foo1.bin upload failed"
    exit 1
fi

curl -T ./tests/data/foo2.bin "tftp://${host}"
curl --output ./tests/tmp/foo2.bin "tftp://${host}/foo2.bin"
if ! diff -q ./tests/data/foo2.bin ./tests/tmp/foo2.bin; then
    killall tftpcpd
    sleep 3
    echo "foo2.bin upload failed"
    exit 2
fi

curl -T ./tests/data/foo3.bin "tftp://${host}"
curl --output ./tests/tmp/foo3.bin "tftp://${host}/foo3.bin"
if ! diff -q ./tests/data/foo3.bin ./tests/tmp/foo3.bin; then
    killall tftpcpd
    sleep 3
    echo "foo3.bin upload failed"
    exit 3
fi

# kill running tftpcpd instance to make sure database persists
killall tftpcpd

sleep 3

./dist/tftpcpd -directory ./tests/root/ -sqlite3-db ./tests/db/tftpcpd.db "${host}" &

sleep 3

curl --output ./tests/tmp/foo1.bin "tftp://${host}/foo1.bin"
if ! diff -q ./tests/data/foo1.bin ./tests/tmp/foo1.bin; then
    killall tftpcpd
    sleep 3
    echo "downloading foo1.bin failed after the server was brought back up"
    exit 4
fi

# replace file in order to check that new version properly registers

mv ./tests/data/foo4.bin ./tests/data/foo1.bin
curl -T ./tests/data/foo1.bin "tftp://${host}"
curl --output ./tests/tmp/foo1.bin "tftp://${host}/foo1.bin"
if ! diff -q ./tests/data/foo1.bin ./tests/tmp/foo1.bin; then
    killall tftpcpd
    sleep 3
    echo "foo1.bin was not updated"
    exit 5
fi

# test tftpcpc download

cd ./tests/tmp/ || exit 6
../../dist/tftpcpc "${host}"/foo1.bin
if ! diff -q ./foo1.bin ../data/foo1.bin; then
    killall tftpcpd
    sleep 3
    echo "tftpcpc client not doing downloads properly"
    exit 7
fi
cd - || exit 8

# test tftpcpc upload

cd ./tests/tmp/ || exit 9
../../dist/tftpcpc -write ../data/foo5.bin "${host}"
../../dist/tftpcpc "${host}"/foo5.bin
if ! diff -q ./foo5.bin ../data/foo5.bin; then
    killall tftpcpd
    sleep 3
    echo "tftpcpc client not doing uploads properly"
    exit 10
fi
cd - || exit 11

sleep 3

killall tftpcpd
echo ""
echo "All passed!"
