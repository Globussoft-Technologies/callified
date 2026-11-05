"""Hush C ABI adapter. One independent native streaming session per call."""
import ctypes
import math
import os

import numpy as np


class HushModel:
    def __init__(self):
        self.lib = ctypes.CDLL(os.getenv('HUSH_LIBRARY', '/opt/hush/libweya_nc.so'))
        lib = self.lib
        lib.weya_nc_model_load_from_path.argtypes = [ctypes.c_char_p]
        lib.weya_nc_model_load_from_path.restype = ctypes.c_void_p
        lib.weya_nc_model_free.argtypes = [ctypes.c_void_p]
        lib.weya_nc_model_free.restype = None
        lib.weya_nc_session_create.argtypes = [ctypes.c_void_p, ctypes.c_size_t, ctypes.c_float]
        lib.weya_nc_session_create.restype = ctypes.c_void_p
        lib.weya_nc_session_free.argtypes = [ctypes.c_void_p]
        lib.weya_nc_session_free.restype = None
        lib.weya_nc_get_frame_length.argtypes = [ctypes.c_void_p]
        lib.weya_nc_get_frame_length.restype = ctypes.c_size_t
        lib.weya_nc_get_input_sample_rate.argtypes = [ctypes.c_void_p]
        lib.weya_nc_get_input_sample_rate.restype = ctypes.c_size_t
        pointer = ctypes.POINTER(ctypes.c_float)
        lib.weya_nc_process_frame.argtypes = [ctypes.c_void_p, pointer, pointer]
        lib.weya_nc_process_frame.restype = ctypes.c_float
        self.handle = lib.weya_nc_model_load_from_path(os.getenv('HUSH_MODEL', '/opt/hush/model.tar.gz').encode())
        if not self.handle:
            raise RuntimeError('Hush model could not be loaded')

    def close(self):
        if self.handle:
            self.lib.weya_nc_model_free(self.handle)
            self.handle = None


class CallProcessor:
    def __init__(self, model, attenuation_db=6.0):
        if not math.isfinite(attenuation_db) or not 0 <= attenuation_db <= 18:
            raise ValueError('HUSH_ATTENUATION_DB must be between 0 and 18')
        self.model = model
        self.session = model.lib.weya_nc_session_create(model.handle, 8000, attenuation_db)
        if not self.session:
            raise RuntimeError('Hush streaming session could not be created')
        self.frame_samples = model.lib.weya_nc_get_frame_length(self.session)
        if not 1 <= self.frame_samples <= 1600 or model.lib.weya_nc_get_input_sample_rate(self.session) != 8000:
            self.close()
            raise RuntimeError('Unsupported Hush frame format')
        self.pending = bytearray()
        self.output = np.zeros(self.frame_samples, dtype=np.float32)

    def close(self):
        if self.session:
            self.model.lib.weya_nc_session_free(self.session)
            self.session = None

    def process(self, pcm):
        self.pending.extend(pcm)
        frame_bytes = self.frame_samples * 2
        output = bytearray()
        consumed = 0
        while len(self.pending) - consumed >= frame_bytes:
            block = self.pending[consumed:consumed + frame_bytes]
            samples = np.frombuffer(block, dtype='<i2').astype(np.float32) / 32768.0
            self.model.lib.weya_nc_process_frame(
                self.session,
                samples.ctypes.data_as(ctypes.POINTER(ctypes.c_float)),
                self.output.ctypes.data_as(ctypes.POINTER(ctypes.c_float)))
            if not np.isfinite(self.output).all():
                raise RuntimeError('Hush produced non-finite audio')
            output.extend(np.clip(np.rint(self.output * 32768), -32768, 32767).astype('<i2').tobytes())
            consumed += frame_bytes
        del self.pending[:consumed]
        return bytes(output)
