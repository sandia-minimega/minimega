# VNC shim test data

`TestProxyNoVNCSession` in `shim_test.go` uses the `novnc-1.7.0-session`
files in this directory. `TestWritePasteFileSample` in `player_test.go` uses
`paste-sample.kb`.

## Files

- `novnc-1.7.0-session.hex`: one client-to-server byte stream, in hex. The
  test ignores whitespace and the text from `#` to the end of a line.
- `novnc-1.7.0-session.kb`: the kb recording lines that the stream must
  give, without the delay prefix (`<delay>:`).
- `paste-sample.kb`: the paste file for the sample text of the test. The
  earlier paste code, which made all lines before it wrote them, wrote this
  file.

## Source of the stream

The stream is synthetic. It is not a capture of a real session. No QEMU and
no browser were available to make a capture.

The stream follows the message order and the encodings of noVNC 1.7.0
(`core/rfb.js`) against a QEMU 6.2 VNC server with no password:

1. The RFB 3.8 handshake with security type None, and a shared ClientInit.
2. SetPixelFormat for a 24-bit screen, and SetEncodings with the 24
   encodings that noVNC 1.7.0 sends when the browser has no H.264 decoder.
   The quality level is 6 and the compression level is 2.
3. FramebufferUpdateRequest messages, a SetDesktopSize to 1280x800, pointer
   events, and QEMU extended key events (255/0) for Shift+L, s, and Return.
4. A second SetEncodings after a change of the quality level to 3, and a
   plain ClientCutText with the text `pwd`.

There is no guest and no capture tool for this stream. A real capture can
show a framing case that the synthetic stream does not have. Replace the
stream with a real capture when you can.

## Make a real capture

Use a test guest that has no secrets. The stream contains all typed and
pasted text.

1. Start minimega with this code, and launch a KVM VM on QEMU 6.2 with
   `vm config bidirectional-copy-paste false`.
2. Get the shim port of the VM from the `vnc_port` column of `vm info`.
3. Start a kb recording: `vnc record kb <vm> /tmp/session.kb`.
4. Start a TCP relay that writes the client-to-server bytes to a file:
   `socat -r /tmp/session.bin TCP-LISTEN:5999,reuseaddr TCP:127.0.0.1:<vnc_port>`.
5. In a clone of the noVNC v1.7.0 tag, run
   `./utils/novnc_proxy --vnc 127.0.0.1:5999 --listen 6080`.
6. Open `http://localhost:6080/vnc.html` in a browser, and set Remote
   Resizing. Connect.
7. Move the mouse and click one time. Type Shift+L, s, and Return.
8. Change the quality level in the settings. Paste the text `pwd` with the
   clipboard panel.
9. Disconnect. Stop the relay. Run `vnc stop kb <vm>`.
10. Write the stream: `xxd -p /tmp/session.bin > novnc-1.7.0-session.hex`.
11. Write the recording lines:
    `cut -d: -f2- /tmp/session.kb > novnc-1.7.0-session.kb`.
12. Change this file: state the guest, the browser, and the date of the
    capture, and remove the text about the synthetic stream.
13. Run `go test -race -run TestProxyNoVNCSession ./internal/vnc`.
