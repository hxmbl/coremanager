# CoreManager

A simple CLI tool to manage CPU cores on Linux.

## Installation

### From source

```bash
go install github.com/hxmbl/coremanager@latest
```

### From release

Download the latest binary from [Releases](https://github.com/Hxmbl/coremanager/releases).

## Usage Examples

### Help

```bash
coremanager --help            # Shows you the help
```

### Turn on or off core

```bash
coremanager dc 2  # Disables 2 cores
coremanager ec 2  # Enables 2 cores
coremanager dc a  # Disable all cores (but the one that can't be)
coremanager ec a  # Enable all cores
```

 `dc` for disabling

`ec` for enabling

Enabling/disabling cores requires root:

```bash
sudo coremanager dc 2
```



The core on and off doesn't turn off an exact core, just a number of them.

### Everything Else

P.S. Alias for coremanager is cm. `make install` creates it for you; if you
installed the binary some other way:

```bash
sudo ln -sf "$(command -v coremanager)" /usr/local/bin/cm
```

```bash
cm cc             # Show the Core Count of active and total
cm cm             # Show the cpu model
cm debug          # Shows debug info (if that's useful to you...)
```



All commands accept `-v`/`--verbose` for detailed output.

`coremanager --version` shows the build version.

## License

MIT — see [LICENSE](LICENSE).
