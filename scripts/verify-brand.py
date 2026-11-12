#!/usr/bin/env python3
from pathlib import Path
import hashlib
root = Path(__file__).resolve().parents[1]
expected = 'c6b175c005ea3a9978d8daa887b77a1243a17de99f9bcea174cc87b2b7046347'
for rel in ['assets/brand/original-mochi-1024.png', 'apps/web/public/icons/tinynest-original.png']:
    actual=hashlib.sha256((root/rel).read_bytes()).hexdigest()
    assert actual==expected, f'ICON HAS CHANGED: {rel} sha256={actual}'
    print(f'OK brand preserved: {rel}')
