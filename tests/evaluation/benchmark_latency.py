"""Warm closed-loop HTTP latency benchmark for fixed voice-assistant requests."""
import argparse
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
import hashlib
import http.client
import json
import math
from pathlib import Path
import threading
import time
from urllib.parse import urlsplit

from evaluate import grade

ROOT = Path(__file__).resolve().parent


def percentile(values, fraction):
    if not values:
        return None
    ordered = sorted(values)
    return ordered[max(0, math.ceil(fraction * len(ordered)) - 1)]


def metrics(records, elapsed, budget):
    successful = [r['latency_ms'] for r in records if r['error'] is None]
    errors = sum(r['error'] is not None for r in records)
    p95 = percentile(successful, .95)
    return dict(attempted=len(records), success=len(successful), errors=errors,
                elapsed_seconds=elapsed, success_qps=len(successful)/elapsed,
                p50_ms=percentile(successful, .5), p95_ms=p95, p99_ms=percentile(successful, .99),
                max_ms=max(successful) if successful else None,
                budget_ms=budget, budget_misses=sum(r['error'] is not None or r['latency_ms'] > budget for r in records),
                meets_budget=errors == 0 and p95 is not None and p95 <= budget)


def validate_response(body, response, model, score_readout='mean'):
    if response.get('model') != model or set(response['answers']) != set(body['questions']):
        raise ValueError('Response model/questions mismatch')
    if response['usage']['input_tokens'] < 1:
        raise ValueError('No encoded input tokens')
    for key, question in body['questions'].items():
        grade({'request': {'questions': {'decision': question}}, 'expected': ''},
              {'model': model, 'answers': {'decision': response['answers'][key]}}, score_readout)


def run_group(url, model, cases, samples, clients, timeout, warmup, score_readout='mean'):
    target = urlsplit(url)
    if target.scheme not in ('http', 'https') or not target.hostname:
        raise ValueError('Use an HTTP(S) endpoint')
    path = target.path or '/'
    if target.query:
        path += '?' + target.query
    local = threading.local()
    connections = []
    lock = threading.Lock()

    def send(index):
        case = cases[index % len(cases)]
        body = dict(case['request'], model=model)
        data = json.dumps(body, ensure_ascii=False).encode()
        if not getattr(local, 'connection', None):
            klass = http.client.HTTPSConnection if target.scheme == 'https' else http.client.HTTPConnection
            local.connection = klass(target.hostname, target.port, timeout=timeout)
            with lock:
                connections.append(local.connection)
        started = time.perf_counter()
        error, tokens = None, None
        raw, status = b'', None
        try:
            local.connection.request('POST', path, body=data, headers={'Content-Type': 'application/json'})
            response = local.connection.getresponse()
            status = response.status
            raw = response.read()
            if response.status != 200:
                raise ValueError('HTTP '+str(response.status)+': '+raw.decode(errors='replace')[:200])
            payload = json.loads(raw)
            validate_response(body, payload, model, score_readout)
            tokens = payload['usage']['input_tokens']
        except (OSError, http.client.HTTPException, ValueError, KeyError, TypeError, AttributeError) as exc:
            if isinstance(exc, http.client.IncompleteRead):
                raw = exc.partial
            error = type(exc).__name__+': '+str(exc)
            local.connection.close()
            local.connection = None
        return dict(case_id=case['id'], latency_ms=(time.perf_counter()-started)*1000,
                    input_tokens=tokens, error=error, http_status=status,
                    response_body=raw.decode('utf-8', errors='replace'))

    try:
        for i in range(warmup):
            result = send(i)
            if result['error']:
                raise ValueError('Warmup failed: '+result['error'])
        with ThreadPoolExecutor(max_workers=clients) as pool:
            started = time.perf_counter()
            records = list(pool.map(send, range(samples)))
            elapsed = time.perf_counter()-started
        return records, elapsed
    finally:
        for connection in connections:
            connection.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--url', required=True)
    parser.add_argument('--model', required=True)
    parser.add_argument('--score-readout', choices=['mean', 'median'], default='mean')
    parser.add_argument('--fixtures', type=Path, default=ROOT/'voice-performance.jsonl')
    parser.add_argument('--clients', nargs='+', type=int, default=[1, 4])
    parser.add_argument('--samples', type=int, default=120, help='Requests per history/question/client group')
    parser.add_argument('--warmup', type=int, default=6)
    parser.add_argument('--timeout', type=float, default=30.)
    parser.add_argument('--budget-ms', type=float, default=100.)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if args.samples < 1 or args.warmup < 0 or any(c < 1 for c in args.clients) or args.timeout <= 0 or args.budget_ms <= 0:
        parser.error('Samples/clients/timeout/budget must be positive, warmup nonnegative')
    cases = [json.loads(line) for line in args.fixtures.read_text().splitlines() if line.strip()]
    if not cases:
        parser.error('No workload fixtures')
    groups = {}
    for case in cases:
        groups.setdefault((case['history_pairs'], len(case['request']['questions'])), []).append(case)
    args.output.mkdir(parents=True, exist_ok=False)
    metadata = dict(started_at=datetime.now(timezone.utc).isoformat(), url=args.url, model=args.model,
                    samples_per_group=args.samples, clients=args.clients, warmup=args.warmup,
                    timeout=args.timeout, budget_ms=args.budget_ms, quantiles='nearest rank of successful requests',
                    score_readout=args.score_readout,
                    workload='closed loop; one persistent HTTP connection per worker; not open-loop production traffic',
                    fixtures_sha256=hashlib.sha256(args.fixtures.read_bytes()).hexdigest(),
                    script_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest())
    (args.output/'metadata.json').write_text(json.dumps(metadata, indent=2)+'\n')
    summary = []
    with (args.output/'samples.jsonl').open('w') as stream:
        for (history, questions), selected in sorted(groups.items()):
            for clients in args.clients:
                records, elapsed = run_group(args.url, args.model, selected, args.samples, clients, args.timeout, args.warmup, args.score_readout)
                key = dict(history_pairs=history, questions=questions, clients=clients)
                for record in records:
                    stream.write(json.dumps(dict(key, **record), ensure_ascii=False)+'\n')
                stream.flush()
                measured = dict(key, **metrics(records, elapsed, args.budget_ms))
                summary.append(measured)
                print(json.dumps(measured), flush=True)
    (args.output/'summary.json').write_text(json.dumps(summary, indent=2)+'\n')
    if any(row['errors'] for row in summary):
        raise SystemExit(1)


if __name__ == '__main__':
    main()
