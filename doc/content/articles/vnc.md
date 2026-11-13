# VNC

minimega can record what a user does at a KVM VM's console, record what the
VM displays, and play keyboard and mouse recordings back into any VM. The
same machinery drives scripted interaction: typing strings, clicking on
things when they appear on screen, and running fleets of "users" with
vncdrone. Use it to capture demonstrations, replay logins and workflows in an
experiment, or turn a session into a video.

This page assumes running KVM VMs (containers and Android VMs have no VNC
console) and, for framebuffer transcoding, `ffmpeg` on the host. Watching a
console interactively is covered in [miniweb](miniweb.md).

## How it works

For every KVM VM except a bare-metal one, minimega listens on a TCP port on
the host running it and proxies that port to QEMU's VNC socket. The port is the `vnc_port` column of
`vm info`; miniweb connects through it. Because every keystroke and mouse
event passes through this proxy, minimega can record input without any agent
in the guest, and its built-in RFB client can connect to the same port to
record the framebuffer or play events back.

Two consequences follow. Recordings are written on the host where the VM runs,
and relative filenames are resolved against that host's `-filepath` directory
(`/tmp/minimega/files` by default). And only input that goes through the proxy
is recorded: a VNC viewer pointed directly at QEMU's UNIX socket is invisible
to `vnc record kb`.

All `vnc` commands are namespace aware. You name the VM and minimega finds the
host.

## Recording

### Keyboard and mouse

```minimega title="vnc3.mm"
--8<-- "articles/vnc/vnc3.mm"
```

[Download this example](vnc/vnc3.mm){ download="vnc3.mm" }

Interact with the VM through miniweb, then stop:

```minimega title="vnc4.mm"
--8<-- "articles/vnc/vnc4.mm"
```

[Download this example](vnc/vnc4.mm){ download="vnc4.mm" }

The file is plain text, one event per line:

```text
<delay>:PointerEvent,<button mask>,<x>,<y>
<delay>:KeyEvent,<pressed>,<key>
```

The delay is the time to wait before sending the event, counted from the
previous one. Always use integer nanoseconds. The playback parser also accepts a Go
duration such as `5s`, but the `time` column of `vnc` adds up the delays with
an integer parser and shows `0` for the whole file if any line uses one. The button mask
follows the RFB protocol
([RFC 6143 §7.5.5](https://www.rfc-editor.org/rfc/rfc6143.html#section-7.5.5)):
bit 0 is the left button (`1`), bit 1 the middle (`2`), bit 2 the right (`4`),
and bits 3 and 4 scroll up and down; `0` is a plain move. `x` and `y` are
pixels from the top-left corner. A key event is `true` for press and `false`
for release, and the key is an X11 keysym name, so a printable character is
itself and everything else has a name like `Return`, `space` or `Shift_L`
(see [Keys and keysyms](#keys-and-keysyms)).

A click and someone typing `hello` look like this:

```text
178759303:PointerEvent,0,606,44
130044895:PointerEvent,1,606,44
97711488:PointerEvent,0,606,44
578412037:KeyEvent,true,f
8141459:KeyEvent,false,f
111708110:KeyEvent,true,o
10379962:KeyEvent,false,o
69607950:KeyEvent,true,o
102641640:KeyEvent,false,o
```

Lines beginning with `#` followed by a colon are comments; the playback logs
them at the `info` level, which makes `#: logging in` a convenient progress
marker. A line with no colon is skipped with a message at the `debug` level; a line
whose event does not parse is skipped with a message at the `error` level.

### Framebuffer

Framebuffer recording captures what the VM displays, at ten frames per
second, without anyone connected to the console:

```minimega title="vnc1.mm"
--8<-- "articles/vnc/vnc1.mm"
```

[Download this example](vnc/vnc1.mm){ download="vnc1.mm" }

```minimega title="vnc2.mm"
--8<-- "articles/vnc/vnc2.mm"
```

[Download this example](vnc/vnc2.mm){ download="vnc2.mm" }

An `.fb` file is a gzip stream of raw RFB framebuffer updates, each preceded
by a header line giving the nanoseconds since the previous chunk and the chunk
length. It is not a video; use `rfbplay` to view or transcode it.

### Listing and clearing

`vnc` with no arguments lists active recordings and playbacks across the
namespace; `clear vnc` stops all of them.

```text
minimega$ vnc
host | name | type        | time                 | filename
mm1  | desktop | record kb   | 42.1s                | /tmp/minimega/files/recording.kb
mm1  | kiosk   | playback kb | 35.99s remaining     | /tmp/minimega/files/login.kb
```

## Playback

A keyboard and mouse recording can be played into any KVM VM, not just the
one it was recorded on, as long as the screen layout matches well enough for
the coordinates to make sense.

```minimega title="vnc5.mm"
--8<-- "articles/vnc/vnc5.mm"
```

[Download this example](vnc/vnc5.mm){ download="vnc5.mm" }

You can watch, and interfere with, a playback through miniweb. Only one
playback runs per VM at a time. The controls:

```minimega title="vnc6.mm"
--8<-- "articles/vnc/vnc6.mm"
```

[Download this example](vnc/vnc6.mm){ download="vnc6.mm" }

```minimega title="vnc7.mm"
--8<-- "articles/vnc/vnc7.mm"
```

[Download this example](vnc/vnc7.mm){ download="vnc7.mm" }

```minimega title="vnc8.mm"
--8<-- "articles/vnc/vnc8.mm"
```

[Download this example](vnc/vnc8.mm){ download="vnc8.mm" }

```minimega title="vnc9.mm"
--8<-- "articles/vnc/vnc9.mm"
```

[Download this example](vnc/vnc9.mm){ download="vnc9.mm" }

```minimega title="vnc10.mm"
--8<-- "articles/vnc/vnc10.mm"
```

[Download this example](vnc/vnc10.mm){ download="vnc10.mm" }

`getstep` prints the line currently being waited on; `step` sends it
immediately instead of waiting out its delay.

### Targeting several VMs

Every playback command takes the same VM targets as `vm start`: a name, a
comma-separated list, a range or `all`. VMs in the target without a playback
report `kb playback not found`.

```minimega
minimega$ vnc play client[1-10] login.kb
minimega$ vnc pause client1,client2
minimega$ vnc stop all
```

### Typing and injecting events

`vnc type` types a string into a VM, handling Shift for you. It covers
printable ASCII plus tab, newline and carriage return; quote strings that
contain spaces, and note that it refuses to run while a playback is active on
that VM. `vnc inject` sends one event in the recording format without the
delay. If a playback is running the event goes down its connection; otherwise
minimega opens a short-lived connection just for it.

```minimega title="vnc11.mm"
--8<-- "articles/vnc/vnc11.mm"
```

[Download this example](vnc/vnc11.mm){ download="vnc11.mm" }

### LoadFile

A `LoadFile` event plays another file to completion, then resumes the current
one; relative paths are taken from the directory of the file being played.
Injected into a VM with no playback running it simply starts one.

```text
10000000000:LoadFile,reboot_windows.kb
```

```minimega title="vnc12.mm"
--8<-- "articles/vnc/vnc12.mm"
```

[Download this example](vnc/vnc12.mm){ download="vnc12.mm" }

Nesting deeper than ten files is logged as a warning and more than a hundred
is an error.

### WaitForIt and ClickItEvent

`WaitForIt` pauses the playback until a template image appears on the VM's
screen, checking about once a second, and fails the playback if the timeout
passes first. `ClickItEvent` does the same and then sends a left-button press at the centre
of the match; it sends no matching release. The image may be a PNG or JPEG file, given as an absolute path
(it is opened by the minimega process, not relative to the playback file), or
the base64 encoding of the image inline, which keeps a script self-contained.

```text
1000:WaitForIt,10s,/tmp/minimega/files/login-button.png
1000:ClickItEvent,30s,/tmp/minimega/files/ok-button.png
```

!!! note "The parser accepts `ClickItEvent`, not `ClickIt`"
    Older documentation names this event `ClickIt`. The playback parser only
    recognises `WaitForIt` and `ClickItEvent`; a `ClickIt` line is logged as an
    invalid message at the `error` level and skipped.

Matching is done on grayscale copies of the screen and the template, by the
lowest sum of absolute differences, so the template must be a pixel-accurate
crop taken at the same resolution and colour depth. Take it from a
`vm screenshot` of the same VM. Neither event can be injected into a VM that
has no running playback.

## Framebuffer playback with rfbplay

`rfbplay` ships with minimega. It has one flag of its own, `-port` (default
9004), plus the usual logging flags, and two forms:

```text
rfbplay [OPTION] <directory>
rfbplay [OPTION] <input.fb> <output>
```

### In a browser

Point it at a directory of `.fb` files and browse to port 9004. The page lists
the files; opening one streams it as motion JPEG, which Firefox plays natively
and Chrome does not. Add `?offset=5m` to a file's URL to start part-way
through.

```bash
$ rfbplay /tmp/minimega/files
```

### Transcoding to video

With an input file and an output file, `rfbplay` serves the recording on its
port and runs `ffmpeg` against it, so `ffmpeg` must be on the `PATH`. The
output format follows the extension:

```bash
$ rfbplay recording.fb recording.mp4
```

The recording is fed to `ffmpeg` at its original rate, so an hour of
recording takes an hour to transcode. Run several at once with different
`-port` values; each takes little CPU. `-level info` shows the `ffmpeg`
output.

## Scripting cookbook

Recordings are text, so the fastest way to build an automated session is to
record the hard parts and generate the rest.

### Keys and keysyms

Key names come from X11's `keysymdef.h`, embedded in minimega as
`internal/vnc/keysymdef.go`. Names are case sensitive. The ones you will use
most:

```text
space BackSpace Tab Return Escape Delete Insert Home End
Left Right Up Down Page_Up Page_Down F1 ... F12
Shift_L Control_L Alt_L Super_L Caps_Lock
minus equal bracketleft bracketright semicolon apostrophe grave
backslash comma period slash
exclam at numbersign dollar percent asciicircum ampersand asterisk
parenleft parenright underscore plus braceleft braceright bar
colon quotedbl less greater question asciitilde
```

A digit or lowercase letter is its own name (`a`, `7`); an uppercase letter is
its uppercase name (`A`). Keys that need Shift on a US keyboard, including
uppercase letters and the symbols in the last three lines above, must be
wrapped in a `Shift_L` press and release or the guest sees the unshifted key.
minimega's own list of shifted keysyms is `internal/vnc/shiftdefs.go`,
generated from QEMU's `en-us` keymap, and `vnc type` applies it
automatically:

```text
50000000:KeyEvent,true,Shift_L
50000000:KeyEvent,true,colon
50000000:KeyEvent,false,colon
50000000:KeyEvent,false,Shift_L
```

Every press needs a matching release, or the guest treats the key as held
down. Chords are presses in order and releases in reverse:

```text
50000000:KeyEvent,true,Alt_L
50000000:KeyEvent,true,F4
50000000:KeyEvent,false,F4
50000000:KeyEvent,false,Alt_L
```

### Typing a string from a script

`vnc type` is enough interactively. When you are assembling a `.kb` file,
generate the events instead:

```python title="genkeys.py"
#!/usr/bin/env python3
"""Emit KeyEvents that type a string: genkeys.py 'P@ss w0rd' >> session.kb"""
import sys

NAMES = {" ": "space", ".": "period", ",": "comma", "/": "slash",
         "\\": "backslash", "-": "minus", "=": "equal", ";": "semicolon",
         "'": "apostrophe", "[": "bracketleft", "]": "bracketright",
         "`": "grave", "\n": "Return", "\t": "Tab"}
SHIFTED = {"!": "exclam", "@": "at", "#": "numbersign", "$": "dollar",
           "%": "percent", "^": "asciicircum", "&": "ampersand",
           "*": "asterisk", "(": "parenleft", ")": "parenright",
           "_": "underscore", "+": "plus", "{": "braceleft", "}": "braceright",
           "|": "bar", ":": "colon", '"': "quotedbl", "<": "less",
           ">": "greater", "?": "question", "~": "asciitilde"}
DELAY = 50_000_000  # 50 ms between events

def events(text):
    for c in text:
        shift = c.isupper() or c in SHIFTED
        name = SHIFTED.get(c) or NAMES.get(c, c)
        if shift:
            yield f"{DELAY}:KeyEvent,true,Shift_L"
        yield f"{DELAY}:KeyEvent,true,{name}"
        yield f"{DELAY}:KeyEvent,false,{name}"
        if shift:
            yield f"{DELAY}:KeyEvent,false,Shift_L"

print("\n".join(events(sys.argv[1])))
```

To type a whole file, loop over its lines and pass each to `events`.

### Mouse events and waits

Surround a click with plain moves so the guest cursor is in place before the
button goes down and released afterwards:

```text
50000000:PointerEvent,0,100,669
50000000:PointerEvent,1,100,669
50000000:PointerEvent,0,100,669
```

There is no wait event. A pointer event with no buttons at the current
position and a long delay is the idiom:

```text
#: wait ten seconds for the desktop
10000000000:PointerEvent,0,0,0
```

### Replacing mouse movement with a single wait

A recorded session is mostly cursor movement. This collapses each run of
button-less moves into one move to the final position, preceded by the
combined delay, which shrinks files and makes them easier to edit by hand.
It expects integer delays, as recordings have.

```python title="shrink.py"
#!/usr/bin/env python3
"""Merge runs of mouse movement in a recording: shrink.py in.kb > out.kb"""
import sys

wait, last = 0, "0,0"
for line in open(sys.argv[1]):
    line = line.rstrip("\n")
    delay, _, event = line.partition(":")
    if event.startswith("PointerEvent,0,"):
        wait += int(delay)
        last = event.split(",", 2)[2]
        continue
    if wait:
        print(f"{wait}:PointerEvent,0,{last}")
        wait = 0
    print(line)
if wait:
    print(f"{wait}:PointerEvent,0,{last}")
```

### Finding coordinates

The recording itself is the easiest source of coordinates: record yourself
clicking the target once and read the `PointerEvent` line. For a screenshot
to measure against, or to crop a template from, save the VM's framebuffer at
full size:

```minimega
minimega$ vm screenshot desktop file /tmp/minimega/files/desktop.png
```

Open it in any image editor that shows the cursor position.

### Template matching outside minimega

`WaitForIt` does the matching for you during a playback. When you want the
same check from your own tooling, for example to decide which recording to
play next, OpenCV's `matchTemplate` on a `vm screenshot` gives a score and a
location. Install `python3-opencv` on the host.

```python title="match.py"
#!/usr/bin/env python3
"""Locate a template in a screenshot: match.py screen.png button.png"""
import sys
import cv2

screen = cv2.imread(sys.argv[1])
template = cv2.imread(sys.argv[2])
result = cv2.matchTemplate(screen, template, cv2.TM_CCOEFF_NORMED)
_, score, _, (x, y) = cv2.minMaxLoc(result)
if score < 0.8:
    sys.exit("not found")
h, w = template.shape[:2]
print(f"{x + w // 2},{y + h // 2}")
```

The printed centre is what you would put in a `PointerEvent`.

### Bulk transcoding

Each `rfbplay` transcode needs its own port, so give every job one:

```python title="transcode.py"
#!/usr/bin/env python3
"""Transcode every .fb under a directory to mp4, four at a time: transcode.py fb/ out/"""
import pathlib
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor

src, dst = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])

def transcode(port, fb):
    out = dst / fb.relative_to(src).with_suffix(".mp4")
    out.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run(["rfbplay", f"-port={port}", str(fb), str(out)], check=True)

with ThreadPoolExecutor(max_workers=4) as pool:
    for i, fb in enumerate(sorted(src.rglob("*.fb"))):
        pool.submit(transcode, 10000 + i, fb)
```

### Keeping only the interesting parts of a video

A VM that sits idle for most of an experiment produces an hour of video with
a minute of activity. This samples a frame every ten seconds, keeps the
ten-second spans where the screen changed, stamps each with its offset and
joins them. It needs `python3-opencv` and an `ffmpeg` built with the
`drawtext` filter.

```python title="motion.py"
#!/usr/bin/env python3
"""Keep only the spans of a video where the screen changes: motion.py in.mp4 out.mp4"""
import pathlib
import subprocess
import sys
import tempfile
import cv2

src, dst = sys.argv[1], sys.argv[2]
work = pathlib.Path(tempfile.mkdtemp())
ff = ["ffmpeg", "-loglevel", "error"]

subprocess.run(ff + ["-i", src, "-vf", "fps=1/10", str(work / "%05d.png")], check=True)
frames = sorted(work.glob("*.png"))
clips = []
for i in range(len(frames) - 1):
    a, b = cv2.imread(str(frames[i])), cv2.imread(str(frames[i + 1]))
    _, score, _, _ = cv2.minMaxLoc(cv2.matchTemplate(b, a, cv2.TM_CCOEFF_NORMED))
    if score < 0.99:
        clip = work / f"clip{i:05d}.mp4"
        subprocess.run(ff + ["-ss", str(i * 10), "-t", "10", "-i", src, "-vf",
                             f"drawtext=text='{i * 10}s':fontsize=20:fontcolor=red:x=10:y=0",
                             str(clip)], check=True)
        clips.append(clip)

(work / "list.txt").write_text("".join(f"file '{c}'\n" for c in clips))
subprocess.run(ff + ["-f", "concat", "-safe", "0", "-i", str(work / "list.txt"),
                     "-c", "copy", dst], check=True)
```

## Simulated users with vncdrone

vncdrone keeps a population of VMs busy by playing recordings into them at
random. It talks to minimega over the command socket, so run it on a host in
the mesh with the matching `-base`. Its flags are `-recordings`, the absolute
path of a directory of `.kb` files, and `-base` (default `/tmp/minimega`),
plus the usual logging flags.

```bash
$ vncdrone -recordings /home/user/recordings/
```

Give the directory with a trailing slash: vncdrone builds each file's path
from the parent of the value you pass, so without the slash it looks one
directory too high.

File names must have exactly three underscore-separated fields:

```text
<vm prefix>_<pre|run|post>_<name>.kb
```

The prefix selects VMs by name prefix (`win_pre_login.kb` plays on `win1`
and `winserver`, not `nowin`). The middle field puts the file in one of three
groups, played in the cycle pre, run, post, pre: typically one `pre` file that
logs in, several `run` files that do work and can be played in any order, and
one `post` file that logs out. After each file the drone has a one in ten
chance of moving to the next group, except that a group with a single `pre`
or `post` file always moves on after playing it.

Every ten seconds vncdrone lists `vm info`, skips any VM that already has a
playback or recording running (from `vnc`), and starts a randomly chosen file
from the current group on each idle VM with `vnc play`. VMs whose names match
no recording prefix are left alone.

## See also

- [miniweb](miniweb.md) for the console you record through.
- [VM lifecycle](vm-lifecycle.md) for `vm screenshot` and VM targets.
- [Command and control](cc.md) when an agent in the guest is a better fit than
  driving the console.
- Reference: [`vnc`](../reference/minimega.md#vnc),
  [`clear vnc`](../reference/minimega.md#clear-vnc),
  [`vm screenshot`](../reference/minimega.md#vm-screenshot).
