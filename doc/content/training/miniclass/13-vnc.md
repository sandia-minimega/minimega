# Chapter 13: VNC recording and scripting

Every KVM VM has a console, and minimega sits between that console and
whoever is using it. In this lab you record the keystrokes sent to
`vm_right`, record what its screen shows while a recording is played back
into it, turn the screen recording into a video with `rfbplay`, and then
write a short console session by hand and play it. The same mechanism drives
scripted logins and fleets of simulated users.

Prerequisites: Part I, in particular [Chapter 3](03-router-sandwich.md) and
[Chapter 5](05-miniweb.md), which is where you first opened a VM's console in
the browser. The lab records from the prompt so miniweb is optional, but it
is the natural way to make your own recordings.

## Rebuild the sandwich

Start from the sandwich with fixed client addresses, as in
[chapter 11](11-traffic.md#rebuild-the-sandwich-with-fixed-addresses); the
script at the end rebuilds it. Only `vm_right` is used, but a console session
that pings `10.0.0.10` is more interesting on a network that works.

## How the console is wired

For each KVM VM, minimega listens on a TCP port on the host, the `vnc_port`
column of `vm info`, and proxies it to QEMU's VNC socket. miniweb connects
through that port, and so does minimega's own RFB client when it plays a
recording or records the framebuffer. Every keyboard and mouse event that
passes through the proxy can be recorded, without any agent in the guest.
Recordings are written on the host that runs the VM; a relative file name
goes to `/tmp/minimega/files`.

## Record what is typed

Start a keyboard and mouse recording on `vm_right`, then send it some input.
Open the console in miniweb and type, or stay at the prompt: `vnc type`
types a string, taking care of Shift for you, and `vnc inject` sends one raw
event, here a press and release of Return:

```minimega
minimega[sandwich]$ vnc record kb vm_right session.kb
minimega[sandwich]$ vnc type vm_right "ping -c 3 10.0.0.10"
minimega[sandwich]$ shell sleep 1
minimega[sandwich]$ vnc inject vm_right KeyEvent,true,Return
minimega[sandwich]$ vnc inject vm_right KeyEvent,false,Return
minimega[sandwich]$ .annotate false vnc
name     | type      | time  | filename
vm_right | record kb | 4.02s | /tmp/minimega/files/session.kb
minimega[sandwich]$ shell sleep 5
minimega[sandwich]$ vnc stop kb vm_right
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
event. Keys are X11 keysym names, so letters and digits are themselves and
`space`, `minus`, `period` and `Return` appear by name; a capital letter or a
shifted symbol is wrapped in `Shift_L` press and release events. Pointer
events look like `PointerEvent,<buttons>,<x>,<y>`. The file is plain text,
which is what makes it editable.

## Record the screen and play the session back

A framebuffer recording captures what the VM displays, ten frames a second,
whether or not anyone is watching. Start one, play the keyboard recording
into the same VM, and stop when it is done:

```minimega
minimega[sandwich]$ vnc record fb vm_right session.fb
minimega[sandwich]$ vnc play vm_right session.kb
minimega[sandwich]$ .annotate false vnc
name     | type        | time            | filename
vm_right | record fb   | 2.51s           | /tmp/minimega/files/session.fb
vm_right | playback kb | 1.08s remaining | /tmp/minimega/files/session.kb
minimega[sandwich]$ shell sleep 10
minimega[sandwich]$ vnc stop fb vm_right
```

A playback can be paused and resumed (`vnc pause`, `vnc continue`),
advanced one event at a time (`vnc step`, with `vnc getstep` showing the
event it is waiting on), or ended early (`vnc stop vm_right`). It can also be
played into any other KVM VM: `vnc play vm_left session.kb` types the same
command into `vm_left`. Only one playback runs per VM at a time, and
`vnc type` refuses to run while one is active.

An `.fb` file is not a video. `rfbplay`, installed with the other tools under
`/opt/minimega/bin` (`bin/rfbplay` in a source build), either serves a
directory of recordings to a browser on port 9004 or transcodes one file with
`ffmpeg`, which must be on the host's `PATH`:

```bash
$ /opt/minimega/bin/rfbplay /tmp/minimega/files/session.fb session.mp4
```

Transcoding runs at the recording's own speed, so a minute of recording takes
a minute. To check what the console shows right now without a recording,
save a screenshot: `vm screenshot vm_right file /tmp/minimega/files/vm_right.png`.

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
minimega[sandwich]$ vnc play vm_right console.kb
```

There is no wait event; a button-less `PointerEvent` with a long delay is
the idiom, as at the top of the file. A login is the same file with the
username and password typed first and a wait for the prompt in between; on
an image with a graphical login, `WaitForIt` and `ClickItEvent` pause the
playback until a template image appears on screen. The
[scripting cookbook](../../articles/vnc.md#scripting-cookbook) has the keysym
names, a generator for typing arbitrary strings, and the template-matching
events. To keep many VMs busy with a library of such recordings, see
[vncdrone](../../articles/vnc.md#simulated-users-with-vncdrone).

## Clean up

`vnc` with no arguments lists every recording and playback in the namespace,
and `clear vnc` stops all of them. The `.kb` and `.fb` files remain in the
files directory.

## What you built

- `session.kb`, a keyboard recording of a command typed into `vm_right`,
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
