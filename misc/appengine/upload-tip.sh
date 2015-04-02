#!/bin/sh
# This updates tip.minimega.org and should be run hourly by our cron job
appcfg.py --oauth2 -A pivotal-sonar-90317 update .
