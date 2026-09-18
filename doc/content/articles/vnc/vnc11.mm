# type the string "hello" into vm desktop
vnc type desktop hello

# the same thing as raw events: each key needs a press and a release
vnc inject desktop KeyEvent,true,h
vnc inject desktop KeyEvent,false,h
vnc inject desktop KeyEvent,true,e
vnc inject desktop KeyEvent,false,e
vnc inject desktop KeyEvent,true,l
vnc inject desktop KeyEvent,false,l
vnc inject desktop KeyEvent,true,l
vnc inject desktop KeyEvent,false,l
vnc inject desktop KeyEvent,true,o
vnc inject desktop KeyEvent,false,o
