# binary-patch

DISCLAIMER: This is only a test!

## Why this?

It should be possible to update client binaries without being
dependent, that all users have to find and download updates manually.

A `mytool --self-update` should be enough or an "autoupdate" feature
should be possible.

I found https://github.com/inconshreveable/go-update and
had an idea.

## Idea

A service could run and host all binaries for one organization.
Binaries should be created by CI build systems for all platforms and
uploaded to a storage system (NFS, ObjectStorage (S3, GCS, ..)).

The clients can send name, architecture and current version to the
service and the service can provide an updated version of the binary
for the same architecture. All client binaries have to have a small
modification to request a new version. This could be done implicitly
to support "autoupdate" or explicitly "--self-update" a user request
based update procedure.


## Examples

### Signed Updates

Create /tmp/bindata/ with 1 client application with versions v0.0.1
and v0.0.2 and generate corresponding signature and sha256 checksum
files:

    % testdata/create_testdata.sh

Build server binary:

    % make build.server

Run example:

    % build/binary-patch version
    binary-patch Version: v0.0.1
    ================================
        Buildtime: 2017-06-14_09:26:33PM
        GitHash: 0edf55ffe02f054090d81b61b84d0b9b2242b92e
    % build/binary-patch signed-patch-update
    2017/06/14 23:27:55 use http://localhost:8080/signed-patch-update
    % build/binary-patch version
    binary-patch Version: v0.0.2
    ================================
        Buildtime: 2017-06-14_09:26:37PM
        GitHash: 0edf55ffe02f054090d81b61b84d0b9b2242b92e

### Upload new version with signature

Current server storage:

    % ls -l /tmp/bindata
    total 15624
    -rwxr-xr-x 1 sszuecs sszuecs 7988426 Jul 10 21:26 binary-patch_v0.0.1_amd64linux
    -rw-r--r-- 1 sszuecs sszuecs      65 Jul 10 21:26 binary-patch_v0.0.1_amd64linux.sha256
    -rw-r--r-- 1 sszuecs sszuecs      72 Jul 10 21:26 binary-patch_v0.0.1_amd64linux.signature
    -rwxr-xr-x 1 sszuecs sszuecs 7988426 Jul 10 21:26 binary-patch_v0.0.2_amd64linux
    -rw-r--r-- 1 sszuecs sszuecs      65 Jul 10 21:26 binary-patch_v0.0.2_amd64linux.sha256
    -rw-r--r-- 1 sszuecs sszuecs      71 Jul 10 21:26 binary-patch_v0.0.2_amd64linux.signature

Create a new Version of your application

    % make build.local VERSION=v0.0.3
    % openssl dgst -sha256 -sign testdata/privateKey build/binary-patch > build/binary-patch.signature
    % openssl dgst -sha256 build/binary-patch | awk '{print $2}' > build/binary-patch.sha256
    % cat build/binary-patch.sha256
    c94b10075d0a7b588748fcfddb0de2239605bc78aa147efcff8d590bddc2ea2a
    % ls -l build/
    total 22288
    -rwxr-xr-x 1 sszuecs sszuecs  7988426 Jul 10 21:28 binary-patch
    -rwxr-xr-x 1 sszuecs sszuecs 14822023 Jul 10 21:26 binary-patch-server
    -rw-r--r-- 1 sszuecs sszuecs       65 Jul 10 21:32 binary-patch.sha256
    -rw-r--r-- 1 sszuecs sszuecs       71 Jul 10 21:31 binary-patch.signature

Upload to PUT /upload/:name endpoint:

    # create json file, because curl is not able to have huge parameters (base64 of a binary as paramater)
    % echo "{\"data\": \"$(cat build/binary-patch | base64)\", \"version\": \"v0.0.3\", \"arch\": \"amd64\", \"os\": \"linux\", \"signature-type\": \"ecdsa\", \"signature\": \"$(cat build/binary-patch.signature | base64)\"}" > my.json
    % less my.json
    % curl -X PUT -H"content-type: application/json" -d @my.json http://localhost:8080/upload/binary-patch
    {"message":"uploaded signed application 'binary-patch' version v0.0.3 for OS linux and architecture amd64"}

Check SHA256 in server target directory is the same as the above calculated on the client:

      % cat /tmp/bindata/binary-patch_v0.0.3_amd64linux.sha256
      c94b10075d0a7b588748fcfddb0de2239605bc78aa147efcff8d590bddc2ea2a

Use your v0.0.1 or v0.0.2 binary to update it (here we create a new v0.0.1):

    % make build.local VERSION=v0.0.1
    % cp -a build/binary-patch{,.v1}  # make a copy to test unverified an verified updates
    % build/binary-patch version
    binary-patch Version: v0.0.1
    ================================
        Buildtime: 2017-07-10_08:21:37PM
        GitHash: 4d2f875f4261b0ea24381089a4965865c4c4079d
    % build/binary-patch update
    2017/07/10 22:18:47 Update complete
    % build/binary-patch version
    binary-patch Version: v0.0.3
    ================================
        Buildtime: 2017-07-10_08:01:21PM
        GitHash: 4d2f875f4261b0ea24381089a4965865c4c4079d
    # Worked!
    #
    # Reset to check signed updates
    % cp -a build/binary-patch{.v1,}
    % build/binary-patch version
    binary-patch Version: v0.0.1
    ================================
        Buildtime: 2017-07-10_08:21:37PM
        GitHash: 4d2f875f4261b0ea24381089a4965865c4c4079d
    % build/binary-patch signed update --public-key testdata/publicKey
    % build/binary-patch version
    binary-patch Version: v0.0.3
    ================================
        Buildtime: 2017-07-10_08:01:21PM
        GitHash: 4d2f875f4261b0ea24381089a4965865c4c4079d
    # Worked!
