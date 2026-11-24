# VNC

minimega can record what a user does at the console of a KVM VM. It can also
record what the VM displays. It can play keyboard and mouse recordings back
into any VM. The same machinery drives scripted interaction. You can type
strings, click on items after they appear on screen, and run fleets of "users"
with vncdrone. Use it to capture demonstrations, replay logins and workflows in
an experiment, or turn a session into a video.

This page assumes running KVM VMs (containers and Android VMs have no VNC
console) and, for framebuffer transcoding, `ffmpeg` on the host.
[miniweb](miniweb.md) covers how to watch a console interactively.

## How it works

For every KVM VM except a bare-metal VM, minimega listens on a TCP port on the
host that runs the VM. minimega proxies that port to the VNC socket of QEMU.
The port is the `vnc_port` column of `vm info`. miniweb connects through it.
Every keystroke and mouse event passes through this proxy, so minimega can
record input without an agent in the guest. The built-in RFB client of minimega
can connect to the same port to record the framebuffer or play events back.

Two consequences follow. minimega writes recordings on the host where the VM
runs. It resolves relative filenames against the `-filepath` directory of that
host (`/tmp/minimega/files` by default). Also, minimega records only input that
goes through the proxy. A VNC viewer pointed directly at the UNIX socket of
QEMU is invisible to `vnc record kb`.

All `vnc` commands are namespace aware. You name the VM and minimega finds the
host.

## Recording

When you kill or flush a VM, or when the guest shuts down, minimega stops all
recordings of that VM and closes their files.

### Keyboard and mouse

```minimega title="vnc3.mm"
--8<-- "articles/vnc/vnc3.mm"
```

[Download this example](vnc/vnc3.mm){ download="vnc3.mm" }

Interact with the VM through miniweb. Then stop the recording:

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
previous event. Always use integer nanoseconds. The playback parser also
accepts a Go duration such as `5s`.

The button mask follows the RFB protocol
([RFC 6143 §7.5.5](https://www.rfc-editor.org/rfc/rfc6143.html#section-7.5.5)):

- Bit 0 is the left button (`1`).
- Bit 1 is the middle button (`2`).
- Bit 2 is the right button (`4`).
- Bits 3 and 4 scroll up and down (`8`, `16`).
- Bits 5 and 6 scroll left and right (`32`, `64`).
- Bits 7 and 8 are the Back and Forward buttons (`128`, `256`).
- `0` is a plain move.

The mask has 9 bits, so its maximum value is `511`. `x` and `y` are pixels from
the top-left corner.

A key event is `true` for press and `false` for release. The key is an X11
keysym name. A printable character is itself. Everything else has a name like
`Return`, `space`, or `Shift_L` (see [Keys and keysyms](#keys-and-keysyms)).

Some clients, such as noVNC, send QEMU extended key events. These events
contain a keysym and the physical position of the key (the scancode).
minimega records each extended key event as a `KeyEvent` line with its keysym.
The recording does not keep the scancode. minimega does not record key events
with keysym 0.

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

Lines that begin with `#` followed by a colon are comments. The playback logs
them at the `info` level, so `#: logging in` is a useful progress marker. The
playback skips a line with no colon and logs a message at the `debug` level. It
skips a line whose event does not parse and logs a message at the `error`
level.

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

The `stop` command ends the recording at once. The file is then a complete
gzip stream.

An `.fb` file is a gzip stream of raw RFB framebuffer updates. A header line
precedes each update. The header line gives the nanoseconds since the previous
chunk and the chunk length. The file is not a video. Use `rfbplay` to view or
transcode it.

Each update request asks for the full screen. If the guest changes its screen
size, the recording continues at the new size.

### Listing and clearing

`vnc` with no arguments lists active recordings and playbacks across the
namespace. `clear vnc` stops all of them.

```text
minimega$ vnc
host | name    | type        | time                    | filename
mm1  | desktop | record kb   | 42.103856214s           | /tmp/minimega/files/recording.kb
mm1  | kiosk   | playback kb | 35.991204377s remaining | /tmp/minimega/files/login.kb
```

## Playback

If the screen layout matches well enough for the coordinates to make sense, you
can play a keyboard and mouse recording into any KVM VM. It does not have to
be the VM that you recorded it on.

```minimega title="vnc5.mm"
--8<-- "articles/vnc/vnc5.mm"
```

[Download this example](vnc/vnc5.mm){ download="vnc5.mm" }

Playback sends classic key events with keysyms. QEMU translates these keysyms
with its default `en-us` keymap. Thus, a recording made on a guest with a
different keyboard layout can replay differently from how it was typed. See
[Keyboard layout](#keyboard-layout).

Playback cannot send the Back and Forward mouse buttons. It removes them from
the button mask and logs one warning for each playback. A playback stops when
its VNC connection to the VM ends, for example when the VM is killed.

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

`getstep` prints the line that the playback is waiting on. `step` sends the
line immediately instead of waiting out its delay.

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

`vnc type` types a string into a VM and handles Shift for you. It covers
printable ASCII plus tab, newline, and carriage return. Quote strings that
contain spaces. `vnc type` refuses to run while a playback is active on that
VM.

`vnc inject` sends one event in the recording format without the delay. If a
playback is running, the event goes down its connection. Otherwise, minimega
opens a short-lived connection only for the event.

```minimega title="vnc11.mm"
--8<-- "articles/vnc/vnc11.mm"
```

[Download this example](vnc/vnc11.mm){ download="vnc11.mm" }

`vnc type` joins its arguments with one space, so this types `hello world`:

```minimega title="vnc13.mm"
--8<-- "articles/vnc/vnc13.mm"
```

[Download this example](vnc/vnc13.mm){ download="vnc13.mm" }

minimega sends each character as key events, with a 10 ms delay before each
event. It types only printable ASCII characters, Tab, and Enter. It skips all
other characters and logs one warning with the number of skipped characters.

minimega types approximately 50 characters each second. Characters that need
Shift take two times longer, so minimega types approximately 25 of them each
second. Keys that you type during this time mix with the typed text.

One `vnc type` or paste can have a maximum of 65,536 characters. Longer text
fails with an error. For each VM, a maximum of 8 texts can wait. When 8 texts
wait, a new text fails with a "paste queue full" error.

minimega types the text for one VM in order. A new `vnc type` or paste waits
until the previous text is typed. While a `vnc play` playback runs on the VM,
`vnc type` fails with an "already playing" error. Also, `vnc play` fails while
minimega types text on the VM. `vnc stop` stops the text that minimega is
typing and removes the text that waits. A VM kill, a flush, or a guest shutdown
also stops and removes this text.

### LoadFile

A `LoadFile` event plays another file to completion. Then it resumes the current
file. For a `LoadFile` line in a file, minimega takes a relative path from the
directory of that file. You can also inject a `LoadFile` event with `vnc
inject`. For that event, minimega always takes a relative path from the files
directory (`-filepath`), as for `vnc play`. If no playback runs, the injected
event starts a playback.

```text
10000000000:LoadFile,reboot_windows.kb
```

```minimega title="vnc12.mm"
--8<-- "articles/vnc/vnc12.mm"
```

[Download this example](vnc/vnc12.mm){ download="vnc12.mm" }

minimega logs a warning for nesting deeper than ten files. Nesting deeper than
a hundred files is an error.

### WaitForIt and ClickIt

`WaitForIt` pauses the playback until a template image appears on the screen
of the VM. It checks about once each second. If the timeout passes first,
`WaitForIt` fails the playback. `ClickIt` does the same and then clicks at the
center of the match, with a left button press and then a release. minimega also
accepts `ClickItEvent` as a different name for `ClickIt`.

The image can be a PNG or JPEG file, or the base64 encoding of the image
inline. Inline encoding keeps a script self-contained. minimega first tries to
open the value as a file. If that fails, minimega decodes the value as a base64
image. A relative file path is relative to the working directory of minimega,
not to the playback file.

```text
1000:WaitForIt,10s,/tmp/minimega/files/login-button.png
1000:ClickItEvent,30s,/tmp/minimega/files/ok-button.png
```

minimega does the matching on grayscale copies of the screen and the template.
It uses the lowest sum of absolute differences. The template must be a
pixel-accurate crop taken at the same resolution and color depth. Take it from
a `vm screenshot` of the same VM. You cannot inject either event into a VM that
has no running playback.

## Copy and paste

A VNC client can send clipboard text to a VM. The
`vm config bidirectional-copy-paste` setting of the VM selects how minimega
uses this text:

- `false` (the default): minimega types the clipboard text as key events, with
  the same rules as `vnc type`. minimega decodes the text as Latin-1, because
  clients send it in that encoding. Text does not go from the VM to the client.
- `true`: minimega sends the clipboard text to QEMU. The QEMU `qemu-vdagent`
  device and `spice-vdagent` in the guest copy UTF-8 text in both directions.

## Keyboard layout

minimega does not give QEMU a keyboard layout (the `-k` option). Clients
such as noVNC send QEMU extended key events, which contain the physical key
position. The guest keyboard layout then decides the character, as with a
physical keyboard. Thus, non-US layouts, AltGr, and dead keys work when the
guest layout matches your keyboard.

If your keyboard layout is different from the guest layout, the guest uses the
key position, not the key label. For example, with a German keyboard on a US
guest, the key labeled Z types y, and AltGr+Q does not type @. To type the
characters on your keys, set the guest layout to match your keyboard. To make
QEMU translate keysyms instead, add the QEMU option with
`vm config qemu-append -k en-us`.

Recordings, `vnc play`, `vnc type`, and pastes send classic key events with
keysyms. QEMU changes each keysym to a US key position with its default `en-us`
keymap. Thus, these features type the expected characters only on guests with
a US keyboard layout.

## Connection limit

QEMU allows 32 shared VNC clients for each VM. When 32 clients are connected,
QEMU closes each new connection. Each viewer, recording, playback, and paste
opens its own connection, so each one counts toward the limit.

## Framebuffer playback with rfbplay

`rfbplay` ships with minimega. It has one flag of its own, `-port` (default
9004), and the usual logging flags. `rfbplay` has two forms, and `-port`
applies only to the directory form:

```text
rfbplay [OPTION] <directory>
rfbplay [OPTION] <input.fb> <output>
```

### In a browser

Point `rfbplay` at a directory of `.fb` files and browse to port 9004. The page
lists the files. If you open a file, `rfbplay` streams it as motion JPEG.
Firefox plays motion JPEG natively, and Chrome does not. To start part-way
through, add `?offset=5m` to the URL of a file.

```bash
$ rfbplay /tmp/minimega/files
```

### Transcoding to video

With an input file and an output file, `rfbplay` serves the recording on a free
local port. It runs `ffmpeg` against the recording, so `ffmpeg` must be on the
`PATH`. The output format follows the extension:

```bash
$ rfbplay recording.fb recording.mp4
```

`rfbplay` feeds the recording to `ffmpeg` at its original rate, so an hour of
recording takes an hour to transcode. Each transcode takes little CPU. `-level
info` shows the `ffmpeg` output.

## Scripting cookbook

Recordings are text, so the fastest way to build an automated session is to
record the hard parts and generate the rest.

### Keys and keysyms

Key names come from X11's `keysymdef.h`, embedded in minimega as
`internal/vnc/keysymdef.go`. Names are case sensitive. These are the names that
you use most:

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

A digit or lowercase letter is its own name (`a`, `7`). An uppercase letter is
its uppercase name (`A`). Keys that need Shift on a US keyboard include
uppercase letters and the symbols in the last three lines above. Wrap each of
these keys in a `Shift_L` press and release. Otherwise, the guest sees the
unshifted key. The list of shifted keysyms of minimega is
`internal/vnc/shiftdefs.go`. The list comes from the `en-us`
keymap of QEMU, and `vnc type` applies it automatically:

```text
50000000:KeyEvent,true,Shift_L
50000000:KeyEvent,true,colon
50000000:KeyEvent,false,colon
50000000:KeyEvent,false,Shift_L
```

Every press needs a matching release. Otherwise, the guest treats the key as
held down. Chords are presses in order and releases in reverse:

```text
50000000:KeyEvent,true,Alt_L
50000000:KeyEvent,true,F4
50000000:KeyEvent,false,F4
50000000:KeyEvent,false,Alt_L
```

### Typing a string from a script

`vnc type` is enough interactively. To assemble a `.kb` file, generate the
events instead:

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

Surround a click with plain moves. The guest cursor is then in place before the
button goes down. The button is released afterwards:

```text
50000000:PointerEvent,0,100,669
50000000:PointerEvent,1,100,669
50000000:PointerEvent,0,100,669
```

There is no wait event. Instead, use a pointer event with no buttons at the
current position and a long delay:

```text
#: wait ten seconds for the desktop
10000000000:PointerEvent,0,0,0
```

### Replacing mouse movement with a single wait

A recorded session is mostly cursor movement. This script collapses each run of
button-less moves into one move to the final position. The one move has the
combined delay before it. This shrinks files and makes them easier to edit by
hand. The script expects integer delays, as recordings have.

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

The recording itself is the easiest source of coordinates. Record yourself
clicking the target once and read the `PointerEvent` line. Save the
framebuffer of the VM at full size. You can then measure coordinates on it or
crop a template from it:

```minimega
minimega$ vm screenshot desktop file /tmp/minimega/files/desktop.png
```

Open it in any image editor that shows the cursor position.

### Template matching outside minimega

`WaitForIt` does the matching for you during a playback. You can run the same
check from your own tooling, for example to decide which recording to play
next. Use `matchTemplate` of OpenCV on a `vm screenshot`. It gives a score and
a location. Install `python3-opencv` on the host.

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

The printed center is the value that you put in a `PointerEvent`.

### Bulk transcoding

Transcodes do not share a port, so you can run several at once:

```python title="transcode.py"
#!/usr/bin/env python3
"""Transcode every .fb under a directory to mp4, four at a time: transcode.py fb/ out/"""
import pathlib
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor

src, dst = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])

def transcode(fb):
    out = dst / fb.relative_to(src).with_suffix(".mp4")
    out.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run(["rfbplay", str(fb), str(out)], check=True)

with ThreadPoolExecutor(max_workers=4) as pool:
    for fb in sorted(src.rglob("*.fb")):
        pool.submit(transcode, fb)
```

### Keeping only the interesting parts of a video

A VM that sits idle for most of an experiment produces an hour of video with
a minute of activity. This script samples a frame every ten seconds.
It keeps the ten-second spans where the screen changed. It stamps each span
with its offset and joins the spans. It needs `python3-opencv` and an `ffmpeg` built with the
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
the mesh with the matching `-base`. Its flags are `-recordings` (the absolute
path of a directory of `.kb` files), `-base` (default `/tmp/minimega`), and
the usual logging flags.

```bash
$ vncdrone -recordings /home/user/recordings/
```

Give the directory with a trailing slash. vncdrone builds the path of each file
from the parent of the value that you pass. Without the slash, vncdrone looks
one directory too high.

File names must have exactly three fields, in this form:

```text
<vm prefix>_<pre|run|post>_<name>.kb
```

The prefix selects VMs by name prefix (`win_pre_login.kb` plays on `win1`
and `winserver`, not `nowin`). The middle field puts the file in one of three
groups. The drone plays the groups in the cycle pre, run, post, pre.

Typically, one `pre` file logs in. Several `run` files do work, and the drone
can play them in any order. One `post` file logs out.

After each file, the drone has a one in ten chance of moving to the next group.
A group with a single `pre` or `post` file is an exception. The drone always
moves on after it plays that file.

Every ten seconds, vncdrone lists `vm info` and skips any VM that already has a
playback or recording running (from `vnc`). Then vncdrone starts a randomly
chosen file from the current group on each idle VM with `vnc play`. vncdrone
leaves alone the VMs whose names match no recording prefix.

## See also

- [miniweb](miniweb.md) for the console you record through.
- [VM lifecycle](vm-lifecycle.md) for `vm screenshot` and VM targets.
- [Command and control](cc.md) for cases in which an agent in the guest is a
  better fit than driving the console.
- Reference: [`vnc`](../reference/minimega.md#vnc),
  [`clear vnc`](../reference/minimega.md#clear-vnc),
  [`vm screenshot`](../reference/minimega.md#vm-screenshot).
