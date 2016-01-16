# magneto

[![Circle CI](https://circleci.com/gh/jfrazelle/magneto.svg?style=svg)](https://circleci.com/gh/jfrazelle/netns)

Pipe runc events to a stats TUI (Text User Interface).

**Usage**

```console
$ sudo runc events | magneto
CPU USAGE    MEM USAGE / LIMIT     MEM %     NET I/O               BLOCK I/O        PIDS
1.28         108.8 MB / 7.902 GB   1.38%     54.86 MB / 792.8 kB   26.64 MB / 0 B   4
```

```console
$ magneto --help
                                  _
 _ __ ___   __ _  __ _ _ __   ___| |_ ___
| '_ ` _ \ / _` |/ _` | '_ \ / _ \ __/ _ \
| | | | | | (_| | (_| | | | |  __/ || (_) |
|_| |_| |_|\__,_|\__, |_| |_|\___|\__\___/
                 |___/

 Pipe runc events to a stats TUI (Text User Interface).
 Version: v0.1.0

  -d    run in debug mode
  -v    print version and exit (shorthand)
  -version
        print version and exit
```

