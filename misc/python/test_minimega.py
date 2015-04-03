#!/usr/bin/env python
'''
Copyright (2015) Sandia Corporation.
Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
the U.S. Government retains certain rights in this software.

Devin Cook <devcook@sandia.gov>

Minimega Python bindings unittests

These tests can be run under Python's unittest framework.

    ./test_minimega.py

Unfortunately, things may start failing if the cli changes.
'''

import os
import unittest
import subprocess
from time import sleep

minimega = None

SCRIPT_DIR = os.path.dirname(os.path.realpath(__file__))
MINIMEGA_BIN = os.path.realpath(os.path.join(SCRIPT_DIR, '..', '..',
                                             'bin', 'minimega'))
MINIMEGA_PROC = None


class TestMinimega(unittest.TestCase):

    @classmethod
    def setUpClass(cls):
        global MINIMEGA_PROC, minimega
        print('generating python API file...')
        subprocess.check_call(
            '{} -cli | {} > {}'.format(
                MINIMEGA_BIN,
                os.path.join(SCRIPT_DIR, 'genapi.py'),
                os.path.join(SCRIPT_DIR, 'minimega.py'),
            ), shell=True)
        minimega = __import__('minimega')
        print('starting minimega...')
        MINIMEGA_PROC = subprocess.Popen(MINIMEGA_BIN,
                                         stdout=subprocess.PIPE,
                                         stdin=subprocess.PIPE,
                                         stderr=subprocess.PIPE)
        sleep(1) #let minimega start up

    @classmethod
    def tearDownClass(cls):
        global MINIMEGA_PROC
        print('killing minimega')
        MINIMEGA_PROC.communicate(b'quit\n')

    def setUp(self):
        try:
            self.mm = minimega.connect('/tmp/minimega/minimega')
        except FileNotFoundError:
            raise minimega.Error('failed to connect?')
        # uncomment the following line to enable debug output:
        #self.mm._debug = True

    def test_stringArgs(self):
        resp = self.mm.vm.config.qemuoverride('add', 'foo', 'bar')
        self.assertEqual('', resp[0]['Response'])
        resp = self.mm.vm.config.qemuoverride('delete', 'all')
        self.assertEqual('', resp[0]['Response'])

    def test_listArgs(self):
        resp = self.mm.echo(['hello', 'there'])
        self.assertEqual('hello there', resp[0]['Response'])
        self.assertRaises(minimega.ValidationError,
                          self.mm.echo, ('hello there',))

    def test_noArgs(self):
        resp = self.mm.bridge()
        self.assertEqual('', resp[0]['Response'])
        self.assertRaises(minimega.ValidationError,
                          self.mm.bridge, ('foo',))

    def test_missingArgs(self):
        self.assertRaises(minimega.ValidationError,
                          self.mm.echo, ())

    def test_optionArgs(self):
        resp = self.mm.clear.capture('pcap')
        self.assertEqual('', resp[0]['Response'])
        self.assertRaises(minimega.ValidationError,
                          self.mm.clear.capture, ('notpcap',))

    def test_optionalArgs(self):
        resp = self.mm.mesh.degree('1')
        self.assertEqual('', resp[0]['Response'])
        resp = self.mm.mesh.degree()
        self.assertEqual('1', resp[0]['Response'])


if __name__ == '__main__':
    unittest.main()
