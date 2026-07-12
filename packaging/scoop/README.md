# Scoop manifest template

CXRE does not publish a Scoop bucket yet. `cxre.json.tmpl` is a ready-to-fill
manifest for a future `rcmcsweeney/scoop-bucket` repository.

For each stable release:

1. Copy `cxre.json.tmpl` to `bucket/cxre.json` in the bucket repository.
2. Replace every `{{VERSION}}` with the release number without the leading `v`.
3. Replace `{{SHA256}}` with the SHA-256 entry for
   `cxre_VERSION_Windows_x86_64.zip` from the release's `checksums.txt`.
4. Confirm the result is valid JSON and install it locally:

   ```powershell
   scoop install .\bucket\cxre.json
   cxre --version
   ```

5. Commit the manifest only after its checksum and GitHub provenance verify.

Once the bucket is public, users will be able to run:

```powershell
scoop bucket add rcmcsweeney https://github.com/rcmcsweeney/scoop-bucket
scoop install rcmcsweeney/cxre
```

Until that repository exists, do not advertise the Scoop command as a supported
installation method. The executable still requires Codex CLI 0.143.0 or newer
and an existing ChatGPT sign-in.
