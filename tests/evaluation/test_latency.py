import unittest

from benchmark_latency import percentile, metrics, validate_response


class LatencyTests(unittest.TestCase):
    def test_nearest_rank_tail_includes_slowest_sample(self):
        self.assertEqual(percentile(list(range(1, 101)), .95), 95)
        self.assertEqual(percentile([10, 20, 999], .99), 999)

    def test_errors_cannot_make_budget_pass_and_stay_in_miss_rate(self):
        result = metrics([{'latency_ms': 30., 'error': None},
                          {'latency_ms': 1., 'error': 'HTTP 422'}], elapsed=1., budget=100.)
        self.assertEqual(result['attempted'], 2)
        self.assertEqual(result['errors'], 1)
        self.assertEqual(result['budget_misses'], 1)
        self.assertFalse(result['meets_budget'])

    def test_multidecision_response_validates_every_question_and_model(self):
        body = {'questions': {
            'a': {'type': 'noul', 'criteria': {'false': 'no', 'true': 'yes'}},
            'b': {'type': 'choice', 'criteria': {'x': 'one', 'y': 'two'}}}}
        response = {'model': 'candidate', 'usage': {'input_tokens': 10}, 'answers': {
            'a': {'type': 'noul', 'noul': .9},
            'b': {'type': 'choice', 'choice': 'x', 'probabilities': {'x': .8, 'y': .2}}}}
        validate_response(body, response, 'candidate')
        response['answers']['b']['choice'] = 'y'
        with self.assertRaises(ValueError):
            validate_response(body, response, 'candidate')
        response['answers']['b']['choice'] = 'x'
        with self.assertRaises(ValueError):
            validate_response(body, response, 'other-model')


if __name__ == '__main__':
    unittest.main()
