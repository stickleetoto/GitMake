# GitMake v1.2.2 Test Report

## Result

**PASS** for the v1.2.2 Authless Self-Upgrade patch.

## Core verification

```text
go test ./...        PASS
go vet ./...         PASS
go test -race ./...  PASS
```

## Regression verification

```text
V0100_GUIDED_UX_E2E_PASS
V100_TOKENLESS_STABILITY_E2E_PASS
V110_CHAT_APPROVAL_E2E_PASS
V120_ONE_SHOT_PUBLISH_E2E_PASS
V121_PROTOCOL_ROUTING_E2E_PASS
```

## v1.2.2 updater coverage

The updater tests verify that:

- public release discovery uses HTTPS without an Authorization header;
- release asset download works through the public release path;
- non-HTTPS or non-GitHub download hosts are rejected;
- SHA-256 package verification still rejects tampering;
- semantic version comparison handles patch/minor/major ordering;
- when the local version is newer than the latest published release, no asset download is attempted.

## Scope note

In-place self-replacement remains Windows x64 only. Linux and macOS release packages are still built for manual installation.
