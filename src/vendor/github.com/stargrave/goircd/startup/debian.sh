#! /bin/sh

### BEGIN INIT INFO
# Provides:          goircd
# Required-Start:    $remote_fs $network
# Required-Stop:     $remote_fs $network
# Default-Start:     2 3 4 5
# Default-Stop:
# Short-Description: goircd IRC server
# Description:       goircd IRC server
### END INIT INFO

NAME=goircd
DAEMON=/home/goircd/goircd
PIDFILE=/var/run/goircd.pid

. /lib/lsb/init-functions

case "$1" in
  start)
    log_daemon_msg "Starting $NAME daemon" "$NAME"
    start-stop-daemon --start --quiet --background \
      --pidfile $PIDFILE --make-pidfile --exec $DAEMON \
      -- -hostname irc.example.com
    log_end_msg $?
    ;;
  stop)
    log_daemon_msg "Stopping $NAME daemon" "$NAME"
    start-stop-daemon --stop --quiet --oknodo --pidfile $PIDFILE
    log_end_msg $?
    rm -f $PIDFILE
    ;;
  restart)
    $0 stop || :
    $0 start
    ;;
  status)
    status_of_proc -p $PIDFILE "$NAME" "$NAME"
    ;;
  *)
    echo "Usage: /etc/init.d/goircd {start|stop|restart|status}"
    exit 1
esac
