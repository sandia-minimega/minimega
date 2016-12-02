/*
goircd -- minimalistic simple Internet Relay Chat (IRC) server
Copyright (C) 2014-2016 Sergey Matveev <stargrave@stargrave.org>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package goircd

var (
	version   string
	hostname  *string
	bind      *string
	motd      *string
	logdir    *string
	statedir  *string
	passwords *string
	tlsBind   *string
	tlsPEM    *string
	verbose   *bool

	LogSink   = logSink
	StateSink = stateSink
)

type Settings struct {
	Hostname  string
	Bind      string
	Motd      string
	Logdir    string
	Statedir  string
	Passwords string
	TlsBind   string
	TlsPEM    string
	Verbose   bool
}

func SetSettings(settings Settings) {
	hostname = &settings.Hostname
	bind = &settings.Bind
	motd = &settings.Motd
	logdir = &settings.Logdir
	statedir = &settings.Statedir
	passwords = &settings.Passwords
	tlsBind = &settings.TlsBind
	tlsPEM = &settings.TlsPEM
	verbose = &settings.Verbose
}
