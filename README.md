# VS Code Cooldown Proxy

A lightweight proxy that filters VS Code extension versions by publish age to mitigate supply-chain attacks.

This ensures you only install extensions that have been publicly available for a cooldown period (e.g., 7 days).
The vast majority of supply-chain attacks get detected within the first 7-14 days,
so while not a panacea, this method is very effective.

This proxy runs on your own machine and transparently proxies all extension marketplace requests,
omitting any recent versions according to the configured proxy URL.

The proxy is intentionally written in pure-Go with no external dependencies,
to avoid further supply-chain attacks through those.

## Install

```bash
go install github.com/dinvlad/vscode-cooldown@v0.1.1 # or use @latest at your own risk
```

## Configuration

In VS Code and VS Code-based IDEs like Cursor, navigate to "Gallery Service URL" in Settings,
and replace the stock URL with either:

- `http://127.0.0.1:8787/vscode/7d` (for VS Code Marketplace)
- `http://127.0.0.1:8787/openvsx/7d` (for Open VSX Marketplace)

Here, `7d` stands for a 7-day cooldown. You can use other values in the format of `14d` (for days) and `4h` (for hours).

## Usage

Start the proxy before opening your editor:

```bash
vscode-cooldown
```

The proxy listens on port 8787 by default.

## Warning

**Please Note**: while [Go's version immutability](https://go.dev/blog/supply-chain) really helps secure releases,
you are encouraged to fork this repo and check its code,
so you're not dependent on my own security as a developer.
