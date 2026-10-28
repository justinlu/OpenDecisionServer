"""Small fixed native-media diagnostics; synthetic speech is not a real-speaker benchmark."""
import argparse
import base64
from collections import defaultdict
import hashlib
import http.client
import json
import math
from pathlib import Path
import struct
import subprocess
import time
import urllib.request
import wave

from evaluate import grade

ROOT = Path(__file__).resolve().parent


def prepare(fixtures, directory):
    directory.mkdir(parents=True, exist_ok=True)
    voices = {}
    for case in fixtures:
        name = case.get('spoken') or case.get('audio')
        if not name or not case.get('transcript'):
            continue
        # Text/audio pairs use the same script and one frozen recording.
        if name in voices:
            continue
        aiff = directory/(Path(name).stem+'.aiff')
        subprocess.run(['say', '-v', case['voice'], '-r', '190', '-o', str(aiff), case['transcript']], check=True)
        subprocess.run(['afconvert', '-f', 'WAVE', '-d', 'LEI16@16000', '-c', '1', str(aiff), str(directory/name)], check=True)
        voices[name] = dict(voice=case['voice'], rate=190, transcript=case['transcript'])
    for name, tone in [('silence.wav', False), ('tone.wav', True)]:
        with wave.open(str(directory/name), 'wb') as stream:
            stream.setparams((1, 2, 16000, 0, 'NONE', 'not compressed'))
            stream.writeframes(b''.join(struct.pack('<h', int(8000*math.sin(2*math.pi*880*i/16000)) if tone and 8000 <= i < 24000 else 0)
                                       for i in range(32000)))
    # Same public image as the upstream model card. Download separately; do not disable TLS checks.
    if not (directory/'cats.jpg').exists():
        raise SystemExit('Copy the upstream COCO 000000039769.jpg fixture to '+str(directory/'cats.jpg'))
    files = {}
    for path in sorted(directory.iterdir()):
        if path.suffix not in ('.wav', '.jpg'):
            continue
        entry = dict(sha256=hashlib.sha256(path.read_bytes()).hexdigest(), bytes=path.stat().st_size)
        if path.suffix == '.wav':
            with wave.open(str(path)) as stream:
                entry.update(seconds=stream.getnframes()/stream.getframerate(), sample_rate=stream.getframerate(), channels=stream.getnchannels())
        files[path.name] = entry
    manifest = dict(files=files, speech=voices, warning='Synthetic macOS TTS; two voices, six scripts. No real-speaker accuracy claim.',
                    image_source='http://images.cocodataset.org/val2017/000000039769.jpg',
                    fixtures_sha256=hashlib.sha256((ROOT/'liquid-d1-media.json').read_bytes()).hexdigest())
    (directory/'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2)+'\n')


def request_for(case, directory):
    body = json.loads(json.dumps(case['request']))
    files = []
    for key, mime in [('image', 'image/jpeg'), ('audio', 'audio/wav')]:
        if case.get(key):
            files.append('data:'+mime+';base64,'+base64.b64encode((directory/case[key]).read_bytes()).decode())
    if files:
        body['files'] = files
    if case.get('spoken'):
        body['questions']['cats']['instructions'] = {'type': 'input_audio', 'input_audio': {
            'data': base64.b64encode((directory/case['spoken']).read_bytes()).decode(), 'format': 'wav'}}
    return body


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--prepare', action='store_true')
    parser.add_argument('--media-dir', type=Path, default=ROOT.parent.parent/'dist/liquid-d1/media')
    parser.add_argument('--url')
    parser.add_argument('--model', default='d1-omni-2b')
    parser.add_argument('--output', type=Path)
    parser.add_argument('--image-only', action='store_true')
    parser.add_argument('--repeats', type=int, default=1)
    args = parser.parse_args()
    fixtures = json.loads((ROOT/'liquid-d1-media.json').read_text())
    if args.prepare:
        prepare(fixtures, args.media_dir)
        return
    if not args.url or not args.output or args.repeats < 1:
        parser.error('--url, --output and positive --repeats required')
    if args.image_only:
        fixtures = [f for f in fixtures if f['group'] == 'image']
    args.output.mkdir(parents=True, exist_ok=False)
    records = []
    with (args.output/'predictions.jsonl').open('w') as stream:
        for repeat in range(args.repeats):
            for case in fixtures:
                body = dict(request_for(case, args.media_dir), model=args.model)
                started = time.perf_counter()
                record = dict(id=case['id'], group=case['group'], repeat=repeat, expected=case['expected'])
                try:
                    req = urllib.request.Request(args.url, data=json.dumps(body, ensure_ascii=False).encode(), headers={'Content-Type': 'application/json'})
                    with urllib.request.urlopen(req, timeout=120) as response:
                        payload = json.load(response)
                    record['response'] = payload
                    results = {}
                    for key, expected in case['expected'].items():
                        q = body['questions'][key]
                        result = grade({'request': {'questions': {'decision': q}}, 'expected': expected},
                                       dict(payload, answers={'decision': payload['answers'][key]}), 'median')
                        results[key] = result
                    record.update(results=results, correct=sum(r['correct'] for r in results.values()), error=None)
                except (OSError, http.client.HTTPException, ValueError, KeyError, TypeError) as exc:
                    record.update(correct=0, error=type(exc).__name__+': '+str(exc))
                record['latency_ms'] = (time.perf_counter()-started)*1000
                records.append(record)
                stream.write(json.dumps(record, ensure_ascii=False)+'\n')
                stream.flush()
                print(case['id'], record['correct'], '/', len(case['expected']), record['error'], flush=True)
    groups = defaultdict(list)
    for record in records:
        groups[record['group']].append(record)
    summary = {key: dict(questions=sum(len(r['expected']) for r in rows), correct=sum(r['correct'] for r in rows),
                         request_errors=sum(r['error'] is not None for r in rows),
                         latencies_ms=[r['latency_ms'] for r in rows]) for key, rows in groups.items()}
    (args.output/'summary.json').write_text(json.dumps(summary, ensure_ascii=False, indent=2)+'\n')
    if any(r['error'] for r in records):
        raise SystemExit(1)


if __name__ == '__main__':
    main()
