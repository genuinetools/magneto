# magneto

![make-all](https://github.com/genuinetools/magneto/workflows/make%20all/badge.svg)
![make-image](https://github.com/genuinetools/magneto/workflows/make%20image/badge.svg)
[![GoDoc](https://img.shields.io/badge/godoc-reference-5272B4.svg?style=for-the-badge)](https://godoc.org/github.com/genuinetools/magneto)
[![Github All Releases](https://img.shields.io/github/downloads/genuinetools/magneto/total.svg?style=for-the-badge)](https://github.com/genuinetools/magneto/releases)

Pipe runc events to a stats TUI (Text User Interface).

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**

- [Installation](#installation)
    - [Binaries](#binaries)
    - [Via Go](#via-go)
- [Usage](#usage)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Installation

#### Binaries

For installation instructions from binaries please visit the [Releases Page](https://github.com/genuinetools/magneto/releases).

#### Via Go

```console
$ go get github.com/genuinetools/magneto
```

## Usage

```console
$ sudo runc events <container_id> | magneto
CPU %   MEM USAGE / LIMIT       MEM %     NET I/O               BLOCK I/O        PIDS
1.84%   108.8 MiB / 3.902 GiB   1.38%     54.86 MB / 792.8 kB   26.64 MB / 0 B   4
```

![chrome.png](chrome.png)

**Usage with the `docker-runc` command that ships with docker**

```console
$ sudo docker-runc -root /run/docker/runtime-runc/moby events <container_id> | magneto
CPU %               MEM USAGE / LIMIT   MEM %               NET I/O             BLOCK I/O           PIDS
100.12%             452KiB / 8EiB       0.00%               0B / 0B             0B / 0B             2
```

```console
$ magneto -h
magneto -  Pipe runc events to a stats TUI (Text User Interface).

Usage: magneto <command>

Flags:

  -d  enable debug logging (default: false)

Commands:

  version  Show the version information.
```

**NOTE:** Almost all this is the exact same as `docker stats`, so thanks to
everyone who made that possible.
