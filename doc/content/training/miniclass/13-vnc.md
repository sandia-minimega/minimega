# Chapter 13: VNC recording and scripting

Every KVM VM has a console, and minimega sits between that console and the
user of the console. In this lab you record the keystrokes that you send to
`vm-right`. Then you record what its screen shows while minimega plays a
recording back into it. You turn the screen recording into a video with
`rfbplay`. Finally, you write a short console session by hand and play it. The
same mechanism drives scripted logins and fleets of simulated users.

Prerequisites: Part I, in particular [Chapter 3](03-router-sandwich.md) and
[Chapter 5](05-miniweb.md). You first opened the console of a VM in the
browser in Chapter 5. The lab records from the prompt, so miniweb is optional.
miniweb is the natural way to make your own recordings.

## Rebuild the sandwich

Start from the sandwich with fixed client addresses, as in
[chapter 11](11-traffic.md#rebuild-the-sandwich-with-fixed-addresses). The
script at the end rebuilds it. The lab uses only `vm-right`, but a console
session that pings `10.0.0.10` is more interesting on a network that works.

## How the console is wired

For each KVM VM, minimega listens on a TCP port on the host and proxies the
port to the VNC socket of QEMU. The `vnc_port` column of `vm info` shows this
port. miniweb connects through that port. When it plays a recording or records the
framebuffer, the RFB client of minimega also connects through that port.

minimega can record every keyboard and mouse event that passes through the
proxy, without any agent in the guest. minimega writes recordings on the host
that runs the VM. A relative file name goes to `/tmp/minimega/files`.

## Record what is typed

Start a keyboard and mouse recording on `vm-right`. Then send it some input.
Open the console in miniweb and type, or stay at the prompt. `vnc type` types
a string and handles Shift for you. `vnc inject` sends one raw event. Here,
the event is a press and release of Return:

```minimega
minimega[sandwich]$ vnc record kb vm-right session.kb
minimega[sandwich]$ vnc type vm-right "ping -c 3 10.0.0.10"
minimega[sandwich]$ shell sleep 1
minimega[sandwich]$ vnc inject vm-right KeyEvent,true,Return
minimega[sandwich]$ vnc inject vm-right KeyEvent,false,Return
minimega[sandwich]$ .annotate false vnc
name     | type      | time         | filename
vm-right | record kb | 4.021563187s | /tmp/minimega/files/session.kb
minimega[sandwich]$ shell sleep 5
minimega[sandwich]$ vnc stop kb vm-right
```

The client image boots straight into a root shell on its console, so the
command runs as soon as Return arrives. Look at what was recorded:

```minimega
minimega[sandwich]$ shell cat /tmp/minimega/files/session.kb
```

```text
1203441:KeyEvent,true,p
812:KeyEvent,false,p
1054:KeyEvent,true,i
...
1077231944:KeyEvent,true,Return
19827:KeyEvent,false,Return
```

One event per line: a delay in nanoseconds since the previous event, then the
event. Keys are X11 keysym names. Letters and digits are themselves, and
`space`, `minus`, `period`, and `Return` appear by name. A capital letter or
a shifted symbol is wrapped in `Shift_L` press and release events. Pointer
events look like `PointerEvent,<buttons>,<x>,<y>`. The file is plain text, so
you can edit it.

## Record the screen and play the session back

A framebuffer recording captures what the VM displays, ten frames a second,
whether or not anyone is watching. Start one and play the keyboard recording
into the same VM. When the playback is done, stop the framebuffer recording:

```minimega
minimega[sandwich]$ vnc record fb vm-right session.fb
minimega[sandwich]$ vnc play vm-right session.kb
minimega[sandwich]$ .annotate false vnc
name     | type        | time                   | filename
vm-right | playback kb | 1.083310524s remaining | /tmp/minimega/files/session.kb
vm-right | record fb   | 2.512846093s           | /tmp/minimega/files/session.fb
minimega[sandwich]$ shell sleep 10
minimega[sandwich]$ vnc stop fb vm-right
```

You can pause and resume a playback with `vnc pause` and `vnc continue`. You
can advance a playback one event at a time with `vnc step`. `vnc getstep`
shows the event that the playback is waiting on. You can end a playback early
with `vnc stop vm-right`.

You can also play a recording into any other KVM VM.
`vnc play vm-left session.kb` types the same command into `vm-left`. Only one
playback runs for each VM at a time. `vnc type` refuses to run while a
playback is active.

An `.fb` file is not a video. `rfbplay` is installed with the other tools
under `/opt/minimega/bin` (`bin/rfbplay` in a source build). It either serves
a directory of recordings to a browser on port 9004 or transcodes one file
with `ffmpeg`. `ffmpeg` must be on the host's `PATH`:

```bash
$ /opt/minimega/bin/rfbplay /tmp/minimega/files/session.fb session.mp4
```

Transcoding runs at the recording's own speed, so a minute of recording takes
a minute. To see what the console shows right now without a recording, save
a screenshot: `vm screenshot vm-right file /tmp/minimega/files/vm-right.png`.

## Write a session by hand

Because recordings are text, the quickest way to script a console session is
to write the events yourself. This file types `hostname`, waits, and types
`uptime`. Every press needs a release, and `#:` lines are comments that the
playback logs at the `info` level:

```text title="13-console.kb"
--8<-- "training/miniclass/scripts/13-console.kb"
```

[Download this recording](scripts/13-console.kb){ download="13-console.kb" }

Copy it to `/tmp/minimega/files/console.kb` and play it:

```minimega
minimega[sandwich]$ vnc play vm-right console.kb
```

There is no wait event. Instead, use a button-less `PointerEvent` with a long
delay, as at the top of the file. A login is the same file with the username
and password typed first and a wait for the prompt in between. On an image
with a graphical login, `WaitForIt` and `ClickItEvent` pause the playback
until a template image appears on screen. The
[scripting cookbook](../../articles/vnc.md#scripting-cookbook) has the keysym
names, a generator for typing arbitrary strings, and the template-matching
events. To keep many VMs busy with a library of such recordings, see
[vncdrone](../../articles/vnc.md#simulated-users-with-vncdrone).

## Clean up

`vnc` with no arguments lists every recording and playback in the namespace,
and `clear vnc` stops all of them. The `.kb` and `.fb` files remain in the
files directory.

## What you built

- `session.kb`, a keyboard recording of a command typed into `vm-right`,
  and `session.fb`, a framebuffer recording of that command being replayed.
- A hand-written recording, `console.kb`, that runs two commands on the
  console, and the pattern for turning it into a scripted login.
- The `rfbplay` step that turns a framebuffer recording into a video.

## Where to read more

- [VNC](../../articles/vnc.md): [recording](../../articles/vnc.md#recording),
  [playback](../../articles/vnc.md#playback),
  [rfbplay](../../articles/vnc.md#framebuffer-playback-with-rfbplay), the
  scripting cookbook and vncdrone.
- [miniweb](../../articles/miniweb.md) for the console you record through
  interactively.
- Reference: [`vnc`](../../reference/minimega.md#vnc),
  [`clear vnc`](../../reference/minimega.md#clear-vnc),
  [`vm screenshot`](../../reference/minimega.md#vm-screenshot).

## Script

```minimega title="13-vnc.mm"
--8<-- "training/miniclass/scripts/13-vnc.mm"
```

[Download this example](scripts/13-vnc.mm){ download="13-vnc.mm" }
