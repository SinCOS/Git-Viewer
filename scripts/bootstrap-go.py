import concurrent.futures, hashlib, pathlib, urllib.request, tarfile
root = pathlib.Path(__file__).resolve().parents[1]
url = 'https://dl.google.com/go/go1.27.3.darwin-amd64.tar.gz'
size = 71697771
expected = 'b6f8e931e4ab497fcde6da8ae4499a58f9d258e55c8a80824a516eae5950a750'
folder = root / '.tools/downloads'
folder.mkdir(parents=True, exist_ok=True)
def part(i):
    start = i * 4000000
    end = min(size, start + 4000000) - 1
    path = folder / ('go.part.%02d' % i)
    if path.exists() and path.stat().st_size == end-start+1:
        return path
    for attempt in range(4):
        try:
            req = urllib.request.Request(url+'?part='+str(i), headers={'Range': 'bytes=%d-%d' % (start,end)})
            with urllib.request.urlopen(req, timeout=45) as r:
                data = r.read()
            if len(data) == size:
                data = data[start:end+1]
            if len(data) != end-start+1:
                raise ValueError('invalid range size')
            path.write_bytes(data)
            print('Downloaded',i,flush=True)
            return path
        except Exception as e:
            print('Retry',i,attempt,type(e).__name__,flush=True)
    raise RuntimeError('download failed: '+str(i))
with concurrent.futures.ThreadPoolExecutor(max_workers=12) as pool:
    parts = list(pool.map(part, range((size+3999999)//4000000)))
archive = folder / 'go1.27.3.darwin-amd64.tar.gz'
with archive.open('wb') as out:
    for p in parts:
        out.write(p.read_bytes())
assert hashlib.sha256(archive.read_bytes()).hexdigest() == expected, 'SHA256 mismatch'
with tarfile.open(archive) as t:
    t.extractall(root / '.tools')
print('Go installed; SHA256 verified',flush=True)
