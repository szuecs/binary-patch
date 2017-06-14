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


## Example

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
