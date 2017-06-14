#!/bin/bash

#set -x
set -e

case $(uname) in
Darwin )
  PATH="/usr/local/opt/openssl/bin:$PATH"
;;
esac

pushd ${0%/*}
p=`pwd`
test -f privateKey || openssl ecparam -genkey -name prime256v1 -noout -out privateKey
test -f publicKey || openssl ec -in privateKey -pubout -out publicKey

test -d /tmp/bindata || mkdir /tmp/bindata
cd ..
rm build/binary-patch
VERSION=v0.0.2 make build.local
mv build/binary-patch /tmp/bindata/binary-patch_v0.0.2_${GOARCH}${GOOS}
VERSION=v0.0.1 make build.local
cp -a build/binary-patch /tmp/bindata/binary-patch_v0.0.1_${GOARCH}${GOOS}

cd /tmp/bindata
for f in *
do
	if [ ${f##*.} != "sha256" ] && [ ${f##*.} != "signature" ] && [ ${f##*.} != "diff" ]
	then
		echo "$f"
		openssl dgst -sha256 $f | awk '{print $2}' > $f.sha256
		openssl dgst -sha256 -sign $p/privateKey $f > $f.signature
	fi
done

## test
#bsdiff oldexe newexe > patch.diff
# bsdiff binary-patch_v0.0.1_amd64darwin binary-patch_v0.0.2_amd64darwin binary-patch_v0.0.1_v0.0.2_amd64darwin.diff
# f=binary-patch_v0.0.1_v0.0.2_amd64darwin
# openssl dgst -sha256 ${f}.diff | awk '{print $2}' > ${f}.diff.sha256
# openssl dgst -sha256 -sign ../testdata/privateKey ${f}.diff > ${f}.diff.signature

popd
