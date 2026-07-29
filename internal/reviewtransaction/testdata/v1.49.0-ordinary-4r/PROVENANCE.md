# Published v1.49.0 ordinary 4R authority fixture

This fixture is a byte-for-byte copy of the immutable review authority emitted by the published Gentle AI v1.49.0 Linux AMD64 binary.

- Release: `v1.49.0`
- Release URL: `https://github.com/Gentleman-Programming/gentle-ai/releases/tag/v1.49.0`
- Archive: `gentle-ai_1.49.0_linux_amd64.tar.gz`
- Archive SHA-256: `0911f76e22446361758c1ece1cb26f37ffc91242a98abc5b122de93ed85a3954`
- Binary SHA-256: `f78aff151c9da49b85d55fba3da410ad17cddff77ecdcec29e13309eb07510de`
- Binary-reported version: `gentle-ai 1.49.0`
- Source reproduction: `/tmp/gentle-ai-1307-runtime-legacy/valid/.git/gentle-ai/review-transactions/v1/legacy-valid`

Canonical authority checksums:

```text
5c6444bb299691060d3d6b449f3177275b02ab472b246d082615b0d851e7b56f  HEAD
e219f2c50ec3c5cf7c83a9844d955511c07041cbfdc9f8530cc6f9bd558d2fa2  artifacts/receipt.json
5608bd6bbd175cd48f0754897f1204e1cae0612d38aeb1af448d5ac4d51c0e9f  events/5608bd6bbd175cd48f0754897f1204e1cae0612d38aeb1af448d5ac4d51c0e9f.json
9b7dc5776fcad044ac56798b9ca3c823b53a3486816c27234ff537dbde2ee0ef  events/9b7dc5776fcad044ac56798b9ca3c823b53a3486816c27234ff537dbde2ee0ef.json
b7d4df583b8e1bb952c6f021e5aeb015cb837cdbf81f827007ca42c29b13278c  events/b7d4df583b8e1bb952c6f021e5aeb015cb837cdbf81f827007ca42c29b13278c.json
bd3ac2bea5b0c51c7205479d680b907b5b88a88c24be899a7cf0e6843d3d23eb  events/bd3ac2bea5b0c51c7205479d680b907b5b88a88c24be899a7cf0e6843d3d23eb.json
d4c310032d9bb4d299277dece13c029b3bae8b9728fa481558c5c2f59d8eed86  events/d4c310032d9bb4d299277dece13c029b3bae8b9728fa481558c5c2f59d8eed86.json
```

`LOCK` is intentionally excluded because it is runtime coordination state rather than canonical authority.
