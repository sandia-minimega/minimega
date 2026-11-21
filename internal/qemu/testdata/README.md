# QEMU help output

The parser tests in `qemu_test.go` use the files in this directory.

## Files

- `qemu-8.2.2-cpu-help.txt`: the output of `qemu-system-x86_64 -cpu help`.
- `qemu-8.2.2-machine-help.txt`: the output of `qemu-system-x86_64 -M help`.
- `qemu-8.2.2-device-help.txt`: the output of `qemu-system-x86_64 -device help`.

## Source of the output

The output is from the QEMU 8.2.2 package of Ubuntu 24.04
(`1:8.2.2+ds-0ubuntu1.18`), without KVM. Thus, the CPU list has no `host`
model. The machine list also has the Ubuntu machine types, for example
`pc-i440fx-noble`, and lists `pc` and `q35` two times.

The files are full output. The trailing spaces at the ends of lines and the
empty line at the end of the CPU output are removed. The parsers split lines
on spaces and stop at the first empty line, so this change has no effect.
