# VNC recording and playback


<a id="TOC_1."></a>

## Introduction

minimega supports recording and playback of both the framebuffer and keyboard
and mouse interactions with VMs. Framebuffer recordings contain VNC/RFB data at
10 frames per second and can be played back in a browser or transcoded to
video, such as mp4, using the `rfbplay` tool.

Keyboard/mouse recordings are stored in a plaintext file format and can be
played back to any running VM. All VNC operations are namespace aware, and
users must only specify the name of the virtual machine and minimega will
automatically locate the host where the virtual machine resides.

<a id="TOC_2."></a>

## Recording

minimega supports recording framebuffer and keyboard/mouse data with the
[`vnc` API](../reference/minimega.md). All recording files are stored
on the host where the virtual machine is currently running. There are a few
caveats to recording data using minimega, depending on on what data you are
recording, described below.

To view current recordings of any kind, simply issue the `vnc` command with no
arguments.

When you kill or flush a VM, or when the guest shuts down, minimega stops all
recordings of that VM and closes their files.

<a id="TOC_2.1."></a>

### Framebuffer

minimega records VM framebuffers (video) by connecting to the target VM using a
built-in VNC/RFB client. minimega can record the framebuffer of VMs running on
any minimega node, so long as it can lookup the VM using `vm info`, and the
remote VM's VNC port is accessible from the minimega node you are issuing the
command from. There is no need to have the web service running, or to be
connected to the VM in order to record framebuffer data.

minimega records framebuffer data at 10 frames per second.

Each update request asks for the full screen. If the guest changes its screen
size, the recording continues at the new size.

For example, to record the framebuffer on VM `bar`, and save to `recording.fb`:

```minimega title="vnc1.mm"
--8<-- "articles/vnc/vnc1.mm"
```

[Download this example](vnc/vnc1.mm){ download="vnc1.mm" }

To stop recording, use the `stop` keyword:

```minimega title="vnc2.mm"
--8<-- "articles/vnc/vnc2.mm"
```

[Download this example](vnc/vnc2.mm){ download="vnc2.mm" }

The `stop` command ends the recording at once. The file is then a complete
gzip stream.

<a id="TOC_2.2."></a>

### Keyboard/mouse

Keyboard and mouse data is recorded in much the same way. For example, to
record keyboard/mouse data on node `foo`, VM `bar`, and save to `recording.kb`
on node `foo`:

```minimega title="vnc3.mm"
--8<-- "articles/vnc/vnc3.mm"
```

[Download this example](vnc/vnc3.mm){ download="vnc3.mm" }

To stop recording:

```minimega title="vnc4.mm"
--8<-- "articles/vnc/vnc4.mm"
```

[Download this example](vnc/vnc4.mm){ download="vnc4.mm" }

The recorded file format uses the following schema:

```text
<time delta>:PointerEvent,<mask>,<x>,<y>
<time delta>:KeyEvent,<press>,<key>
```

The time delta is the time, in nanoseconds, between the previous record and
this one. Users may use a time duration (e.g. "5m3s") if generating these files
manually.

For pointer events, the button mask is the sum of the buttons that are
pressed. A mask of 0 is no buttons, 1 is the left button, 2 the middle button,
and 4 the right button. For example, 5 is both left and right. The values 8
and 16 are scroll up and down, and 32 and 64 are scroll left and right. The
values 128 and 256 are the Back and Forward buttons. The mask has 9 bits, so
its maximum value is 511.

For keyboard events, there is an event for a key press (press is `true` in the
schema), and a key release. For code points not represented by ASCII, the key
value is one of the codepoints defined in the minimega
[keydef file](https://github.com/sandia-minimega/minimega/blob/master/internal/vnc/keysymdef.go).

Some clients, such as noVNC, send QEMU extended key events. These events
contain a keysym and the physical position of the key (the scancode).
minimega records each extended key event as a `KeyEvent` line with its keysym.
The recording does not keep the scancode. minimega does not record key events
with keysym 0.

For example, the following shows several mouse movements, and someone typing
`foo`:

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
436817511:PointerEvent,0,606,43
54109:PointerEvent,0,606,41
4740247:PointerEvent,0,607,38
39063:PointerEvent,0,607,17
```

<a id="TOC_3."></a>

## Playback

<a id="TOC_3.1."></a>

### Framebuffer

Playback of framebuffer data uses a seperate tool, available in the minimega
distribution, `rfbplay`. `rfbplay` can serve a directory of framebuffer files,
and can playback in a MJPEG supported web browser (Firefox currently supports
MJPEG, Chrome no longer does).

Additionally, `rfbplay` can transcode framebuffer data, using `ffmpeg`, to any
format supported by `ffmpeg`, such as mp4.

<a id="TOC_3.1.1."></a>

#### Using a browser

To playback a framebuffer recording in a web browser that supports MJPEG (not
Chrome), start `rfbplay` and supply a directory to serve:

```text
rfbplay <directory>
```

Then simply browse to the rfbplay service, port 9004 by default, and select the
framebuffer recording you want to play.

<a id="TOC_3.1.2."></a>

#### Transcoding to video

To transcode a framebuffer recording, you must have `ffmpeg` in your path.
Simply invoke `rfbplay` with a source framebuffer file and output video.
`ffmpeg` will infer the video type based on the filename extension. For
example, to transcode a file `foo.fb` to an mp4 file named `bar.mp4`, make sure
you suffix the output filename with `.mp4`:

```text
rfbplay foo.fb bar.mp4
```

Files are transcoded in **real time**, so a one hour framebuffer recording will
take at least one hour to transcode. You can see `ffmpeg` transcoding details
by running `rfbplay` with debug logging.

<a id="TOC_3.2."></a>

### Keyboard/mouse

minimega supports playing and interacting with recorded keyboard/mouse data to
any running VM, not just the one it was recorded on. minimega uses a built-in
VNC/RFB client to playback data. To playback data to a VM on a node other than
the node you are issuing the command on, minimega must be able to directly
connect to the VNC server of the VM on that node.

Playback sends classic key events with keysyms. QEMU translates these keysyms
with its default `en-us` keymap. Thus, a recording made on a guest with a
different keyboard layout can replay differently from how it was typed. See
the Keyboard layout section below.

Playback cannot send the Back and Forward mouse buttons. It removes them from
the button mask and logs one warning for each playback. A playback stops when
its VNC connection to the VM ends, for example when the VM is killed.

For example, to playback a recording to VM `bar`:

```minimega title="vnc5.mm"
--8<-- "articles/vnc/vnc5.mm"
```

[Download this example](vnc/vnc5.mm){ download="vnc5.mm" }

Similarly, to stop playback:

```minimega title="vnc6.mm"
--8<-- "articles/vnc/vnc6.mm"
```

[Download this example](vnc/vnc6.mm){ download="vnc6.mm" }

To pause a running playback:

```minimega title="vnc7.mm"
--8<-- "articles/vnc/vnc7.mm"
```

[Download this example](vnc/vnc7.mm){ download="vnc7.mm" }

To resume a paused playback:

```minimega title="vnc8.mm"
--8<-- "articles/vnc/vnc8.mm"
```

[Download this example](vnc/vnc8.mm){ download="vnc8.mm" }

To get the vnc event the playback is currently on:

```minimega title="vnc9.mm"
--8<-- "articles/vnc/vnc9.mm"
```

[Download this example](vnc/vnc9.mm){ download="vnc9.mm" }

To advance the playback to the next vnc event:

```minimega title="vnc10.mm"
--8<-- "articles/vnc/vnc10.mm"
```

[Download this example](vnc/vnc10.mm){ download="vnc10.mm" }

The playback API also supports injecting arbitrary vnc events into a VM. If a
playback is currently running, the event will immediately be delivered through
the existing vnc connection. If no playback exists for the specified VM, a
short lived vnc connection will be created to deliver the vnc event. The format
of the events is identical to the recording format except with the time delta
removed.

To inject the string `foo` string to VM `bar`:

```minimega title="vnc11.mm"
--8<-- "articles/vnc/vnc11.mm"
```

[Download this example](vnc/vnc11.mm){ download="vnc11.mm" }

<a id="TOC_3.2.1."></a>

#### Comments

Lines that begin with a "#" are treated as comments and skipped.

<a id="TOC_3.2.2."></a>

#### Targetting multiple VMs

The vnc playback APIs support targetting playbacks on multiple VMs at once
using the same syntax as the `vm` APIs:

```text
vnc pause foo,bar
vnc pause foo[1-10]
vnc pause all
```

<a id="TOC_3.2.3."></a>

#### LoadFile event

The playback file also supports a `LoadFile` event. LoadFile will take an
existing vnc keyboard/mouse recording and, if a playback is currently running,
preempt the running playback and play the specified playback file to
completion. After the LoadFile playback completes, the previously running
playback will resume and continue playing. If LoadFile is injected with no
playback currently running it will start a new vnc playback with the specified
playback file.

In a playback file, a relative LoadFile path is relative to the directory of
that playback file. In an injected LoadFile event, a relative path is relative
to the minimega `-filepath` directory.

To inject the playback file `playback.kbr` to VM `foo`:

```minimega title="vnc12.mm"
--8<-- "articles/vnc/vnc12.mm"
```

[Download this example](vnc/vnc12.mm){ download="vnc12.mm" }

<a id="TOC_3.2.4."></a>

#### WaitForIt event

Another special event is the `WaitForIt` event. This event causes the playback
to search the VM's screenshot for the template image and continue once the
template image has been found. The event supports a timeout which will cause
the playback to stop if exceeded.

```text
1000:WaitForIt,10s,template.png
```

You may also base64-encode the image and include it in place of the filename.
This allows your VNC scripts to be self-contained.

minimega first tries to open the value as a file. If that fails, minimega
decodes the value as a base64 image. A relative file path is relative to the
working directory of minimega.

<a id="TOC_3.2.5."></a>

#### ClickIt event

Similar to the `WaitForIt` event, the `ClickIt` event waits until a template
image appears in the VM screenshot but has an additional action to click on the
center of the template image in the screenshot.

```text
1000:ClickIt,10s,template.png
```

As with the `WaitForIt` event, you may base64-encode the image to use in place
of the filename.

The click is a left button press and then a release. minimega also accepts
`ClickItEvent` as a different name for `ClickIt`.

<a id="TOC_3.2.6."></a>

#### Typing text

The `vnc type` command types a string on a VM. For example, to type
`hello world` on VM `bar`:

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

<a id="TOC_4."></a>

## Copy and paste

A VNC client can send clipboard text to a VM. The
`vm config bidirectional-copy-paste` setting of the VM selects how minimega
uses this text:

- `false` (the default): minimega types the clipboard text as key events, with
  the same rules as `vnc type`. minimega decodes the text as Latin-1, because
  clients send it in that encoding. Text does not go from the VM to the client.
- `true`: minimega sends the clipboard text to QEMU. The QEMU `qemu-vdagent`
  device and `spice-vdagent` in the guest copy UTF-8 text in both directions.

<a id="TOC_5."></a>

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

<a id="TOC_6."></a>

## Connection limit

QEMU allows 32 shared VNC clients for each VM. When 32 clients are connected,
QEMU closes each new connection. Each viewer, recording, playback, and paste
opens its own connection, so each one counts toward the limit.
