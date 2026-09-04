# Module 36 - VNC Scripting

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-36-vnc-scripting/).

## Introduction

VNC recordings are just plain text files with commands.

## Format

Files can contain: mouse events, keyboard events, load events, and no actionable lines. Anything that does not have proper syntax or has invalid key definitions will be ignored.

### Mouse Events

```
574598:PointerEvent,0,100,669
```

`574598` - Is the time to wait in nanoseconds before doing an action.

Note: Adding 6 decimal places to microseconds will get you nanoseconds. A, 1 and 9 zeros is a second. In this case `574598` is ~0.575ms.

`PointerEvent` - Issues either a mouse click or a click release

`0` - Is a non clicked state. `1` is a clicked state.

`100,669` - Are the x and y mouse coordinates. Where `0,0` is the top left.

When writing mouse events by hand you should surround them with non-click presses as well; that way, the OS can move the mouse to where it needs to be and you can stop clicking.

```
50000000:PointerEvent,0,100,669
50000000:PointerEvent,1,100,669
50000000:PointerEvent,0,100,669
```

### Keyboard Events

```
1000000:KeyEvent,true,a
1000000:KeyEvent,false,a
1000000 - Is the time to wait in nanoseconds before doing an action. (1ms)
```

`KeyEvent` - Issues a either a keydown or keyup event.

`true` - Is a key down event, `false` is a key up event.

### Special Keyboard Keys

Every single key is defined in a map in the source code [link](https://github.com/sandia-minimega/minimega/blob/d18fd1a2343d3fb7665d92620583217ba9738b0b/src/vnc/keysymdef.go)

Here are most of the ones you will use. Note that these keys are case-sensitive.

```
space
BackSpace
Tab
Return
Super_L
Shift_L
Control_L
Alt_L
Escape
Delete
Left
Up
Right
Down
Home
End
Insert
slash
backslash
underscore
bracketleft
bracketright
braceleft
braceright
question
exclam
at
numbersign
dollar
percent
ampersand
parenleft
parenright
asterisk
apostrophe
quotedbl
colon
semicolon
comma
period
minus
grave
less
equal
greater
bar
1
a
F1
```

If a key requires a shift on your keyboard you will likely need to do the same regardless of the key's name. For example, `colon`:

```
50000:KeyEvent,true,Shift_L
50000:KeyEvent,true,colon
50000:KeyEvent,false,colon
50000:KeyEvent,false,Shift_L
```

### LoadFile

The playback file supports a `LoadFile` event. `LoadFile` will take an existing VNC keyboard/mouse recording and, if a playback is currently running, preempt the running playback and play the specified playback file to completion. After the `LoadFile` playback completes, the previously running playback will resume and continue playing. If `LoadFile` is injected with no playback currently running it will start a new VNC playback with the specified playback file.

```
10000000000:LoadFile,/home/john/recordings/reboot_windows.vnc
```

You can inject `LoadFile` events in the same way as other keyboard or mouse events:

```
# inject the LoadFile event to vm foo to play the playback bar.kbr
vnc inject foo LoadFile,bar.kbr
```

### WaitForIt

Another special event is the `WaitForIt` event. This event causes the playback to search the VM's screenshot for the template image and continue once the template image has been found. The event supports a timeout which will cause the playback to stop if exceeded.

```
1000:WaitForIt,10s,template.png
```

You may also base64-encode the image and include it in place of the filename. This allows your VNC scripts to be self-contained.

### ClickIt

Similar to the `WaitForIt` event, the `ClickIt` event waits until a template image appears in the VM screenshot but has an additional action to click on the center of the template image in the screenshot.

```
1000:ClickIt,10s,template.png
```

As with the `WaitForIt` event, you may base64-encode the image to use in place of the filename.

## Tools

Creating these files by hand may be exhausting. Here are some scripts to help out.

### Type a string

```
# genKeys.py
import sys
def vncType(t):
 retstr = ""
 shift = 0
 for c in t:
  shift = 0
  if c == ".":
   c = "period"
  if c == " ":
   c = "space"
  if c == "/":
   c = "slash"
  if c == "\\":
   c = "backslash"
  if c == ":":
   c = "colon"
   shift = 1
  if c == "@":
   c = "at"
   shift = 1
  if c == "!":
   c = "exclam"
   shift = 1
  if c == "#":
   c = "numbersign"
   shift = 1
  if c == "$":
   c = "dollar"
   shift = 1
  if c == "%":
   c = "percent"
   shift = 1
  if c == "&":
   c = "ampersand"
   shift = 1
  if c == "\'":
   c = "apostrophe"
  if c == "(":
   c = "parenleft"
   shift = 1
  if c == ")":
   c = "parenright"
   shift = 1
  if c == "*":
   c = "asterisk"
   shift = 1
  if c == "+":
   c = "plus"
   shift = 1
  if c == ",":
   c = "comma"
  if c == "-":
   c = "minus"
  if c == ";":
   c = "semicolon"
  if c == "<":
   c = "less"
   shift = 1
  if c == ">":
   c = "greater"
   shift = 1
  if c == "?":
   c = "question"
   shift = 1
  if c == "=":
   c = "equal"
  if c == "[":
   c = "bracketleft"
  if c == "]":
   c = "bracketright"
  if c == "{":
   shift = 1
   c = "braceleft"
  if c == "}":
   shift = 1
   c = "braceright"
  if c == "_":
   c = "underscore"
   shift = 1
  if c == "\"":
   c = "quotedbl"
   shift = 1
  if c == "\n":
   c = "Return"
  if c == "\t":
   c = "Tab"
  if shift == 1:
   retstr = retstr +"50000000:KeyEvent,true,Shift_L\n"
  retstr = retstr + "50000000:KeyEvent,true,"+c+"\n50000000:KeyEvent,false,"+c+"\n"
  if shift == 1:
   retstr = retstr +"50000000:KeyEvent,false,Shift_L\n"
 return retstr.rstrip("\n")
print vncType(sys.argv[1])
```

```
# python genKeys.py "P@ss w0rd"
50000000:KeyEvent,true,P
50000000:KeyEvent,false,P
50000000:KeyEvent,true,Shift_L
50000000:KeyEvent,true,at
50000000:KeyEvent,false,at
50000000:KeyEvent,false,Shift_L
50000000:KeyEvent,true,s
50000000:KeyEvent,false,s
50000000:KeyEvent,true,s
50000000:KeyEvent,false,s
50000000:KeyEvent,true,space
50000000:KeyEvent,false,space
50000000:KeyEvent,true,w
50000000:KeyEvent,false,w
50000000:KeyEvent,true,0
50000000:KeyEvent,false,0
50000000:KeyEvent,true,r
50000000:KeyEvent,false,r
50000000:KeyEvent,true,d
50000000:KeyEvent,false,d
```

### Type a file

Add this to the end of `genKeys.py` to type a whole file.

```
with open ("file.txt", "r") as myfile:
    data=myfile.readlines()
for c in data:
    vncType(c)
```

### Replacing mouse movements with merged waits in recordings

```
#!/usr/bin/python

import sys

if len(sys.argv) != 2:
 print "Syntax Error: Expecting\n./shrink.py input_file"
 exit(1)

global timer
timer = 0
global changetimer
changetimer = 1
with open (sys.argv[1], "r") as myfile:
 for line in myfile:
  if 'PointerEvent,0' in line:
   split = line.split(":")
   split = split[0]
   timer = timer + int(split)
  else:
   if 'PointerEvent,1' in line or 'PointerEvent,4' in line:
    if timer == 0:
     print line.rstrip('\n')
     split = line.split(",")
     print ("10000:PointerEvent,0,"+split[-2]+","+split[-1]),
    else:
     split = line.split(",")
     print (str(timer)+":PointerEvent,0,"+split[-2]+","+split[-1]),
     line = line.lstrip('\n')
     print line.rstrip('\n')
     print ("10001:PointerEvent,0,"+split[-2]+","+split[-1]),
   elif 'KeyEvent' in line:
    if timer != 0:
     print str(timer)+":PointerEvent,0,0,0"
     print line.rstrip('\n')
    else:
     print line.rstrip('\n')
   else:
    # looks like a comment
    print line.rstrip('\n')
    changetimer = 0
   if changetimer ==1:
    timer = 0
   else:
    changetimer = 1
 print str(timer)+":PointerEvent,0,0,0"
```

### Printing coordinates as your mouse moves

```
nano minimega/misc/web/novnc/include/input.js
ctrl+w and search for onMouseMove(pos.x
Modify the code to have the nested if/else section added.
```

```
var evt = (e ? e : window.event);
var pos = Util.getEventPosition(e, this._target, this._scale);
    if (this._onMouseMove) {
        if (noVNC_status.innerHTML.indexOf('|')==-1){
            noVNC_status.innerHTML += " | X:"+pos.x+" Y:"+pos.y;
        }
        else{
            noVNC_status.innerHTML=noVNC_status.innerHTML.split("|")[0]+"| X:"+pos.x+" Y:"+pos.y;
        }
        this._onMouseMove(pos.x, pos.y);
    }
```

The result should look like this.

![Image of posbanner.png](../../assets/legacy-site/2022/03/posbanner.png)

### Checking if a smaller picture exists on the screen.

Useful for waiting for something before clicking on it.

```
#apt-get install python-opencv
import cv2
smallimgpath="/home/ubuntu/small.png"
largeimgpath="/home/ubuntu/large.png"
small = cv2.imread(smallimgpath)
large = cv2.imread(largeimgpath)
method = cv2.TM_CCOEFF_NORMED
res = cv2.matchTemplate(large,small,method)
min_val,max_val,min_loc,max_loc=cv2.minMaxLoc(res)
threshold=0.8
if(max_val < threshold):
 print "not found"
else:
 small_h,small_w,small_k=small.shape
 x_center =max_loc[0] +small_w/2
 y_center =max_loc[1] +small_h/2
 print "Center found: "+str(x_center)+","+str(y_center)
```

You can take a screenshot of a vm using vm screenshot

```
vm screenshot myvm file /home/ubuntu/test.png
```

## Example VNC Script

```
# Click on the screen to disable screensaver
50000000:PointerEvent,0,100,750
50000000:PointerEvent,1,100,750
50000000:PointerEvent,0,100,750
# Wait 10s
10000000000:PointerEvent,0,0,0
# Press Windows+r
50000000:KeyEvent,true,Super_L
50000000:KeyEvent,true,r
50000000:KeyEvent,true,r
50000000:KeyEvent,false,Super_L
# Wait 10s
10000000000:PointerEvent,0,0,0
# Type cmd.exe enter
50000000:KeyEvent,true,c
50000000:KeyEvent,true,c
50000000:KeyEvent,true,m
50000000:KeyEvent,true,m
50000000:KeyEvent,true,d
50000000:KeyEvent,true,d
50000000:KeyEvent,true,period
50000000:KeyEvent,true,period
50000000:KeyEvent,true,e
50000000:KeyEvent,true,e
50000000:KeyEvent,true,x
50000000:KeyEvent,true,x
50000000:KeyEvent,true,e
50000000:KeyEvent,true,e
50000000:KeyEvent,true,Return
50000000:KeyEvent,true,Return
# Wait 10s
10000000000:PointerEvent,0,0,0
# Alt+F4
50000000:KeyEvent,true,Alt_L
50000000:KeyEvent,true,F4
50000000:KeyEvent,true,F4
50000000:KeyEvent,false,Alt_L
```

### Authors

The minimega authors

Created: 14 Jun 2017

Last updated: 3 June 2022
