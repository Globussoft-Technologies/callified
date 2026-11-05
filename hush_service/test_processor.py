import unittest
from concurrent.futures import ThreadPoolExecutor
import numpy as np
from processor import CallProcessor, HushModel


class ProcessorTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.model = HushModel()

    @classmethod
    def tearDownClass(cls):
        cls.model.close()

    def collect(self, data, chunk):
        call = CallProcessor(self.model, 6)
        try:
            return b''.join(call.process(data[i:i+chunk]) for i in range(0, len(data), chunk))
        finally:
            call.close()

    def test_frame_boundaries_and_duration(self):
        audio = (1000*np.sin(2*np.pi*440*np.arange(8000)/8000)).astype('<i2').tobytes()
        whole = self.collect(audio, len(audio))
        chunked = self.collect(audio, 137)
        self.assertEqual(whole, chunked)
        self.assertEqual(len(whole), len(audio))

    def test_quiet_telephone_signal_survives(self):
        for amplitude in (30, 100, 1000):
            for frequency in (200, 440, 1000, 2500):
                with self.subTest(amplitude=amplitude, frequency=frequency):
                    original = (amplitude*np.sin(2*np.pi*frequency*np.arange(8000)/8000)).astype('<i2')
                    result = np.frombuffer(self.collect(original.tobytes(), 320), dtype='<i2').astype(float)
                    self.assertGreater(np.sqrt(np.mean(result[4000:]**2)),
                                       .35*np.sqrt(np.mean(original.astype(float)[4000:]**2)))

    def test_call_state_isolation_and_silence(self):
        first = CallProcessor(self.model)
        second = CallProcessor(self.model)
        try:
            first.process(np.full(800, 8000, dtype='<i2').tobytes())
            result = second.process(bytes(1600))
            self.assertEqual(result, bytes(len(result)))
        finally:
            first.close()
            second.close()
            second.close()

    def test_concurrent_calls_have_independent_state(self):
        audio = (500*np.sin(2*np.pi*350*np.arange(1600)/8000)).astype('<i2').tobytes()
        expected = self.collect(audio, 320)
        with ThreadPoolExecutor(max_workers=4) as pool:
            results = list(pool.map(lambda _: self.collect(audio, 137), range(8)))
        for result in results:
            self.assertEqual(result, expected)

    def test_noise_is_reduced(self):
        audio = np.random.default_rng(42).integers(-1000, 1001, 8000, dtype=np.int16)
        result = np.frombuffer(self.collect(audio.astype('<i2').tobytes(), 320), dtype='<i2').astype(float)
        self.assertLess(np.sqrt(np.mean(result[4000:]**2)),
                        .95*np.sqrt(np.mean(audio.astype(float)[4000:]**2)))

    def test_reject_unsafe_suppression_setting(self):
        for value in (-1, 100, float('nan')):
            with self.assertRaises(ValueError):
                CallProcessor(self.model, value)
