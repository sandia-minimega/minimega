# Module 35 - VNC Playback

!!! warning "Historical miniclass"
    This lesson was recovered from the former Sandia minimega site. It is preserved for reference and may describe obsolete software, operating systems, commands, or external resources. Consult the [current documentation](../../index.md) before applying it.

[View the archived source URL](https://www.sandia.gov/minimega/module-35-vnc-playback-2/).

## Introduction

You can playback keyboard and mouse recordings on any VM running in minimega regardless of where it was made.

Framebuffers can be played back in a browser or converted.

## Keyboard and Mouse

You can play back keyboard and mouse actions. They will be played back with the same delays used in creating them.

```
$ vnc play lin1 /home/ubuntu/lin1.vnc
```

You can view running playbacks

```
$ vnc
host | name | type        | time                    | filename
m2   | lin1 | playback kb | 35.991687464s remaining | /home/ubuntu/lin1.kb
```

Note: You can also view the actions live by connecting to the novnc session and you can add input while it is running.

While it is running you can pause the kb/mouse playback with pause

```
$ vnc pause lin1
```

You can continue execution with continue

```
$ vnc continue lin1
```

You can inject new steps (wait 100s at mouse position 0,0)

```
$ vnc inject lin1 100000000000:PointerEvent,0,0,0
```

You can view the current step with getstep

```
$ vnc getstep lin1
```

You can skip over running steps with continue

```
$ vnc step lin1
```

You can stop the playback with stop

```
$ vnc stop lin1
```

## Framebuffer

Playback of framebuffer data uses a separate tool, available in the minimega distribution, rfbplay.

rfbplay can serve a directory of framebuffer files, and can playback in a MJPEG supported web browser (Firefox currently supports MJPEG, Chrome no longer does).

Additionally, rfbplay can transcode framebuffer data, using ffmpeg, to any format supported by ffmpeg, such as mp4.

## Using a browser

To playback a framebuffer recording in a web browser that supports MJPEG (not Chrome), start rfbplay and supply a directory to serve:

rfbplay <directory> Then simply browse to the rfbplay service, port 9004 by default, and select the framebuffer recording you want to play.

## Transcoding to Video

To transcode a framebuffer recording, you must have ffmpeg in your path. Simply invoke rfbplay with a source framebuffer file and output video. ffmpeg will infer the video type based on the filename extension. For example, to transcode a file foo.fb to an mp4 file named bar.mp4, make sure you suffix the output filename with .mp4:

```
rfbplay foo.fb bar.mp4
```

Files are transcoded in real time, so a one hour framebuffer recording will take at least one hour to transcode. You can see ffmpeg transcoding details by running rfbplay with debug logging.

Transcoding is not very cpu intensive and multiple conversions can be done at once.

## Bulk Transcoding

In a multithreaded manner convert frame buffers to mp4 for all .fb files in the fb/ folder.

```
#!/usr/bin/python
import cv2, glob, os, fnmatch, commands

fblist = []
for root, dirnames, filenames in os.walk('fb/'):
    for filename in fnmatch.filter(filenames, '*.fb'):
        fblist.append(os.path.join(root, filename))
fblist.sort()

lines = ""
count2 = 10
for x in fblist:
 infilefb = x
 foldername = x.split("/")[1]+"/"
 infilemp4 = x.split("/")
 infilemp4 = infilemp4[len(infilemp4)-1]
 infilemp4 = infilemp4.split(".fb")[0]+".mp4"
 infilemp4 = foldername + infilemp4
 os.system("mkdir -p out/"+foldername)
 count2 = count2 +1
 lines = lines + "/home/ubuntu/minimega/bin/rfbplay -port=100"
 lines = lines + str(count2)+" "+infilefb+" "+"out/"+infilemp4+"\n"

print lines
def cmd(thecmd):
 print "[+] "+thecmd
 commands.getstatusoutput(thecmd)
count = 1

cmd("tmux kill-session -t convert")
cmd("tmux new -s convert -d")

for line in lines.split("\n"):
 cmd("tmux neww -t convert -n rfb"+str(count))
 cmd("tmux send-keys -t convert:rfb"+str(count)+ " '" +line +"' Enter")
 count = count +1
```

## Shrinking Recordings

Often times a virtual machine will go unused for long portions of time.

I created a python script that takes a folder of mp4's and creates smaller videos with only the sections that contain movement and overlays a timestamp.

A snapshot is taken from the mp4 every 10s with ffmpeg, opencv is then used to detect motion, ffmpeg overlays a timestamp on video clips that have motion, and ffmpeg finally stitches it all together.

```
#!/usr/bin/python
#apt-get install python-opencv
import cv2, glob, os, fnmatch, random, commands

fblist = []
for root, dirnames, filenames in os.walk('fb/'):
    for filename in fnmatch.filter(filenames, '*.fb'):
        fblist.append(os.path.join(root, filename))
fblist.sort()

#alist = []
#alist.append(fblist[len(fblist)-1])
#fblist = alist

for x in fblist:
 infilefb = x
 foldername = x.split("/")[1]+"/"
 infilemp4 = x.split("/")
 infilemp4 = infilemp4[len(infilemp4)-1]
 infilemp4 = infilemp4.split(".fb")[0]+".mp4"
 infilemp4 = foldername + infilemp4

 print "[+] Expecting conversion of "+infilefb+" to out/"+infilemp4
 #os.system("mkdir -p out/"+foldername)
 #os.system("/home/ubuntu/minimega/bin/rfbplay "+infilefb+" "+"out/"+infilemp4)

 print "[+] Generating random number"
 rand = str(random.randint(1,10000000000))
 print "[+] "+rand

 print "[+] Creating random folder img/"+rand
 os.system("mkdir -p img/"+rand)

 print "[+] Creating snapshots every 10s"
 print "ffmpeg -i "+"out/"+infilemp4+" -vf fps=1/10 img/"+rand+"/%05d.jpg"
 os.system("ffmpeg -i "+"out/"+infilemp4+" -vf fps=1/10 img/"+rand+"/%05d.jpg")

 print "[+] Creating random folder vid/"+rand
 os.system("mkdir -p vid/"+rand)

 print "[+] Detecting motion in snapshots and creating 10s videos, this will take a while"
 filelist = glob.glob("/home/ubuntu/shrinker/img/"+rand+"/*.jpg")
 filelist.sort()
 i = 0
 while i < len(filelist):
  if (i+1) < len(filelist):
   smallimgpath=filelist[i]
   largeimgpath=filelist[i+1]
   small = cv2.imread(smallimgpath)
   large = cv2.imread(largeimgpath)
   method = cv2.TM_CCOEFF_NORMED
   res = cv2.matchTemplate(large,small,method)
   min_val,max_val,min_loc,max_loc=cv2.minMaxLoc(res)
   threshold=0.99
   if(max_val < threshold):
    print "different: "+smallimgpath +" != "+largeimgpath
    a = smallimgpath.split("/")
    a = a[len(a)-1]
    a = a.split(".")
    a = a[0]
    b = largeimgpath.split("/")
    b = b[len(b)-1]
    b = b.split(".")
    b = b[0]
    parta = a
    parta = int(parta)*10
    m, s = divmod(parta, 60)
    h, m = divmod(m, 60)
    parta = "%02d:%02d:%02d" % (h, m, s)
    partb = b
    partb = int(partb)*10
    m, s = divmod(partb, 60)
    h, m = divmod(m, 60)
    partb = "%02d:%02d:%02d" % (h, m, s)
    print a +" - " +b
    print parta+ " " + partb
    cmd = "ffmpeg -i out/"+infilemp4+" -ss "+parta +" -to "+ partb +" -flags +global_header vid/"+rand+"/" +a+"-"+b+".mp4"
    print cmd
    print ""
    os.system(cmd)
   #else:
    #print "match found: "+smallimgpath +" == "+largeimgpath
  i = i +1

 print "[+] Creating random folder vidtext/"+rand
 os.system("mkdir -p vidtext/"+rand)

 print "[+] Adding timestamps to 10s video clips"
 filelist = glob.glob("/home/ubuntu/shrinker/vid/"+rand+"/*.mp4")
 filelist.sort()
 for x in filelist:
  y = x.replace("/vid/","/vidtext/")
  a = "ffmpeg -i !!infile -vf drawtext=fontfile=/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf:\"text='!!text':fontsize=20:fontcolor=red:x=10:y=0\" !!outfile"
  name = y.split(".")[0]
  a = a.replace("!!infile",x)
  a = a.replace("!!outfile",name.split("-")[0]+".mp4")
  parta = name.split("/")
  parta = parta[len(parta)-1]
  parta = parta.split("-")[0]
  partb = name.split("/")
  partb = partb[len(partb)-1]
  partb = partb.split("-")[1].split(".")[0]
  parta = int(parta)*10
  partb = int(partb)*10
  m, s = divmod(parta, 60)
  h, m = divmod(m, 60)
  parta = "%dh%02dm%02ds" % (h, m, s)
  m, s = divmod(partb, 60)
  h, m = divmod(m, 60)
  partb = "%dh%02dm%02ds" % (h, m, s)
  name = parta + " to " +partb
  a = a.replace("!!text",name)
  print a
  os.system(a)

 shrinkname = infilemp4.split(".mp4")[0]+"-shrunk.mp4"
 print "[+] Merging video clips to out/"+shrinkname
 cmd = "ffmpeg -f concat -safe 0 -i <(find vidtext/"+rand+"/ -name '*.mp4' -printf \"file '$PWD/%p'\\n\" | sort) -c copy out/"+shrinkname
 with open("temp.sh", "w") as f:
  f.write(cmd)
 os.system("chmod +x temp.sh")
 os.system("bash /home/ubuntu/shrinker/temp.sh")
 os.system("rm temp.sh")
 print "[+] Done\n"
print "[+] All fb processed"
```
