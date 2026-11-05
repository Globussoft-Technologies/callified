"""Fetch pinned upstream artifacts; no executable/model downloads at runtime."""
import hashlib
import pathlib
import urllib.request

ROOT = pathlib.Path('/opt/hush')
ROOT.mkdir(parents=True, exist_ok=True)
COMMIT = '9f6414e91461a8f4bdf9840c0cdcdcb7da986339'
ARTIFACTS = {
    'libweya_nc.so': (
        f'https://raw.githubusercontent.com/pulp-vision/Hush/{COMMIT}/deployment/lib/libweya_nc.so',
        'c4a5915c92e9500a49e6cd14804e6e88319277de2ef7d4d5cf6c0ac4456f3bbf'),
    'model.tar.gz': (
        'https://huggingface.co/weya-ai/hush/resolve/main/onnx/advanced_dfnet16k_model_best_onnx.tar.gz',
        '45632ccaa82b71bb743d6caa7c78e983fe2f2790a3af7f6ec48e6ed7ba085df6'),
}
for name, (url, checksum) in ARTIFACTS.items():
    with urllib.request.urlopen(url, timeout=120) as response:
        data = response.read()
    if hashlib.sha256(data).hexdigest() != checksum:
        raise RuntimeError(f'Upstream checksum changed: {name}')
    (ROOT / name).write_bytes(data)
with urllib.request.urlopen(f'https://raw.githubusercontent.com/pulp-vision/Hush/{COMMIT}/LICENSE', timeout=30) as response:
    (ROOT / 'LICENSE').write_bytes(response.read())
