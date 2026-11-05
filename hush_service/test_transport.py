import json
import subprocess
import sys
import time
import unittest
import urllib.request

from websockets.sync.client import connect


class TransportTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = subprocess.Popen([sys.executable, '-m', 'uvicorn', 'service:app', '--host', '127.0.0.1', '--port', '18003'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        for _ in range(100):
            try:
                with urllib.request.urlopen('http://127.0.0.1:18003/health', timeout=1) as response:
                    cls.health = json.load(response)
                    return
            except OSError:
                time.sleep(.05)
        cls.server.terminate()
        cls.server.wait(timeout=5)
        raise RuntimeError('Hush service did not become healthy')

    @classmethod
    def tearDownClass(cls):
        cls.server.terminate()
        cls.server.wait(timeout=5)

    def test_real_websocket_processes_and_buffers_audio(self):
        self.assertEqual(self.health['model'], 'weya-ai/hush')
        self.assertEqual(self.health['attenuation_db'], 6)
        for _ in range(2):
            with connect('ws://127.0.0.1:18003/process', open_timeout=2) as ws:
                ready = json.loads(ws.recv(timeout=2))
                self.assertTrue(ready['ready'])
                self.assertEqual(ready['sample_rate'], 8000)
                frame_bytes = ready['frame_bytes']
                ws.send(bytes(frame_bytes-1))
                self.assertEqual(ws.recv(timeout=2), b'')
                ws.send(b'\0')
                self.assertEqual(ws.recv(timeout=2), bytes(frame_bytes))
