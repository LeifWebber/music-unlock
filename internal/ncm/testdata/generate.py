"""Generate synthetic NCM regression fixtures from a local sine wave.
Development only: requires openssl and ffmpeg. No account or music downloads.
Container/cipher reference: Unlock Music CLI v0.2.12 (MIT).
"""
import base64
import json
from pathlib import Path
import struct
import subprocess
import zlib

HERE = Path(__file__).resolve().parent
FLAC = HERE.parent.parent / 'qmc/testdata/tone.flac'
CORE = b'hzHRAmso5kInbaxW'
META = b"#14ljk_!\\]&0U<'("

def aes(data, key):
    return subprocess.run(['openssl', 'enc', '-aes-128-ecb', '-K', key.hex()],
                          input=data, capture_output=True, check=True).stdout

def chunk(kind, data):
    return struct.pack('>I',len(data))+kind+data+struct.pack('>I',zlib.crc32(kind+data))

COVER = b'\x89PNG\r\n\x1a\n'+chunk(b'IHDR',struct.pack('>IIBBBBB',1,1,8,2,0,0,0))+chunk(b'IDAT',zlib.compress(b'\0\xff\x80\0'))+chunk(b'IEND',b'')
(HERE/'cover.png').write_bytes(COVER)

def encode(audio, meta=True, cover=True):
    key=b'public-test-fixture-key'
    box=list(range(256)); last=0
    for index in range(256):
        last=(box[index]+last+key[index%len(key)])%256
        box[index],box[last]=box[last],box[index]
    encoded=bytearray(audio)
    for index in range(len(encoded)):
        j=(index+1)%256
        encoded[index]^=box[(box[j]+box[(box[j]+j)%256])%256]
    encrypted=aes(b'neteasecloudmusic'+key,CORE)
    key_data=bytes(b^0x64 for b in encrypted)
    # Format intentionally disagrees with FLAC data: sniff actual audio.
    metadata=json.dumps({'musicName':'测试曲目','artist':[['测试作者',1],['Second',2]],'album':'Synthetic tone','format':'mp3'},ensure_ascii=False).encode()
    meta_data=b"163 key(Don't modify):"+base64.b64encode(aes(b'music:'+metadata,META)) if meta else b''
    meta_data=bytes(b^0x63 for b in meta_data)
    image=COVER if cover else b''
    allocated=len(image)+32
    return (b'CTENFDAM\x01\x6d'+struct.pack('<I',len(key_data))+key_data+
            struct.pack('<I',len(meta_data))+meta_data+b'\0'*5+
            struct.pack('<II',allocated,len(image))+image+b'\0'*32+encoded)

subprocess.run(['ffmpeg','-v','error','-y','-i',str(FLAC),'-map_metadata','-1','-c:a','libmp3lame','-write_xing','0',str(HERE/'tone.mp3')],check=True)
(HERE/'tagged-flac.ncm').write_bytes(encode(FLAC.read_bytes()))
(HERE/'tagged-mp3.ncm').write_bytes(encode((HERE/'tone.mp3').read_bytes()))
(HERE/'no-meta.ncm').write_bytes(encode(FLAC.read_bytes(),False,False))
