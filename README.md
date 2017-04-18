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

    % go build ./cmd/binary-patch-server
    % go build -o binary-patch.v1 ./cmd/binary-patch
    % ./binary-patch.v1 --version
    v0.0.1
    % sed -i 's/v0.0.1/v0.0.2/g' ./cmd/binary-patch/main.go
    % go build ./cmd/binary-patch
    % ./binary-patch-server &
    % ./binary-patch.v1
    Update complete
    % ./binary-patch.v1 --version
    v0.0.2
