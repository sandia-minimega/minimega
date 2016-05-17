import commands, os
out = commands.getstatusoutput('ps aux | grep qemu')
count = 1

os.system('pkill -f websockify')
hostname = commands.getoutput('hostname')
os.system('mkdir -p vnc')

# This is the template for the index.html
html = ""
html = html+"<html><head><style>th,td {\nborder: 1px solid black;\nwidth: 25%;\ntext-align: center;\n}\n</style></head>\n"
html = html+"<body><h1>Recovery Mode</h1><table style='width:100%'>\n"
html = html+"<tr><th>Drive</th><th>Memory</th><th>Snapshot</th><th>VNC</th></tr>\n"

# This is template for the autotiler
auto = """<html><body><h1>Auto Tiler</h1><style>
#wrapper { width: 400px; height: 253px; padding: 0; overflow: hidden; float:left; margin-right: 2px; margin-bottom: 2px;}
#scaled-frame { width: 1000px; height: 633px; border: 0px; }
#scaled-frame {zoom: 0.40;-moz-transform: scale(0.40);-moz-transform-origin: 0 0;-o-transform: scale(0.40);
-o-transform-origin: 0 0;-webkit-transform: scale(0.40);-webkit-transform-origin: 0 0;}
@media screen and (-webkit-min-device-pixel-ratio:0) { #scaled-frame { zoom: 1; }}</style>"""

with open ('vnc.html','r') as f:
    template=f.read()

for line in str(out).split('\\n'):
    #print str(count)+' '+line
    count = count + 1
    memory = "n/a"
    vncport = "n/a"
    drive = "n/a"
    snapshot = "0"
    if line.find('-m ') > -1:
        memory = line[line.find("-m "):].split(' ')[1]
        print 'mem = '+memory
    if line.find(' -vnc 0.0.0.0:') > -1:
        vncport = line[line.find(' -vnc'):].split(':')[1].split(' ')[0]
        print 'vncport = '+vncport
    if line.find('-drive file=') > -1:
        drive = line[line.find('-drive file='):].split('=')[1].split(',')[0]
        print 'drive = '+drive
    if line.find('-snapshot') > -1:
        print 'snapshot = true'
        snapshot = 1
    if snapshot == 1:
        snapshot = "True"
    else:
        snapshot = "False"
    if "n/a" not in vncport:
        with open ('vnc/'+vncport+'.html','w') as f:
            temp = template.replace("!!PORT",str(int(vncport)+12000))
            f.write(temp)
        url = 'http://'+hostname+':8000/vnc/'+vncport+'.html'
        html = html+"<tr><td>"+drive+"</td><td>"+memory+"</td><td>"+snapshot+"</td><td>"+"<a href='"+url+"'>"+vncport+"</a></td></tr>\n"
        os.system('nohup python novnc/utils/websockify/websockify.py --web ./ '+str(int(vncport)+12000)+' localhost:'+str(int(vncport)+5900)+"&")
        auto = auto + '<div id="wrapper"><iframe id="scaled-frame" src="'+url+'"></iframe></div>'

html = html + "</table></br><a style='font-size: 20px' href='auto.html'>AutoTile</a></body></html>"

auto = auto + "</body></html>"

with open ('auto.html','w') as f:
    f.write(auto)

with open ('index.html','w') as f:
    f.write(html)

print "\nControl + C to quit"
print "Run pkill -f websockify to clean up created sockets.\n"
os.system("python -m SimpleHTTPServer")


