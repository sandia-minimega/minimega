# packages

Local forks of third-party libraries that protonuke uses for its FTP and IRC
traffic. `go.mod` points each module here with a `replace` directive, and
`versions` records the upstream commit each fork started from (all zeros: no
recorded upstream commit).

These are not pristine copies. minimega has changed them since they were added
in 2016 and 2017, so fix bugs here and do not replace them with upstream
releases without carrying the changes over (`git log -- packages/` and, before
2022, `git log -- src/vendor/`):

| Package | Used by | Local changes |
|---|---|---|
| `github.com/dutchcoders/goftp` | protonuke FTP client | bug fixes from upstream pull requests that were never merged, hit counting, IPv6 passive data address |
| `github.com/goftp/server` | protonuke FTP server | hit counting, minilog logging, TLS only after an upgrade on passive connections, race fixes |
| `github.com/stargrave/goircd` | protonuke IRC server | converted from a program to a package, hit counting, room locking |
| `github.com/thoj/go-ircevent` | protonuke IRC client | race fixes, hit counting, IPv6 server addresses |
