# magneto

[![Travis CI](https://travis-ci.org/jessfraz/magneto.svg?branch=master)](https://travis-ci.org/jessfraz/magneto)

Pipe runc events to a stats TUI (Text User Interface).

## Installation

#### Binaries

- **darwin** [386](https://github.com/jessfraz/magneto/releases/download/v0.0.0/magneto-darwin-386) / [amd64](https://github.com/jessfraz/magneto/releases/download/v0.0.0/magneto-darwin-amd64)
- **freebsd** [386](https://github.com/jessfraz/magneto/releases/download/v0.0.0/magneto-freebsd-386) / [amd64](https://github.com/jessfraz/magneto/releases/download/v0.0.0/magneto-freebsd-amd64)
- **linux** [386](https://github.com/jessfraz/magneto/releases/download/v0.0.0/magneto-linux-386) / [amd64](https://github.com/jessfraz/magneto/releases/download/v0.0.0/magneto-linux-amd64) / [arm](https://github.com/jessfraz/magneto/releases/download/v0.0.0/magneto-linux-arm) / [arm64](https://github.com/jessfraz/magneto/releases/download/v0.0.0/magneto-linux-arm64)
- **solaris** [amd64](https://github.com/jessfraz/magneto/releases/download/v0.0.0/magneto-solaris-amd64)
- **windows** [386](https://github.com/jessfraz/magneto/releases/download/v0.0.0/magneto-windows-386) / [amd64](https://github.com/jessfraz/magneto/releases/download/v0.0.0/magneto-windows-amd64)

#### Via Go

```bash
$ go get github.com/jessfraz/magneto
```

## Usage

```console
$ sudo runc events | magneto
CPU %   MEM USAGE / LIMIT     MEM %     NET I/O               BLOCK I/O        PIDS
1.84%   108.8 MB / 3.902 GB   1.38%     54.86 MB / 792.8 kB   26.64 MB / 0 B   4
```

![chrome.png](chrome.png)

```console
$ magneto --help
                                  _
 _ __ ___   __ _  __ _ _ __   ___| |_ ___
| '_ ` _ \ / _` |/ _` | '_ \ / _ \ __/ _ \
| | | | | | (_| | (_| | | | |  __/ || (_) |
|_| |_| |_|\__,_|\__, |_| |_|\___|\__\___/
                 |___/

 Pipe runc events to a stats TUI (Text User Interface).
 Version: v0.0.0
 Build: fc18a9b

  -d    run in debug mode
  -v    print version and exit (shorthand)
  -version
        print version and exit
```

**NOTE:** Almost all this is the exact same as `docker stats`, so thanks to
everyone who made that possible.

