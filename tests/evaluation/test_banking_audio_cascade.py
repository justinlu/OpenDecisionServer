import unittest

from banking_audio_cascade import route_success, needs_business


class CascadeTests(unittest.TestCase):
    def test_dispatch_depends_on_prediction_not_gold(self):
        self.assertTrue(needs_business({'predicted':'navigate','error':None}))
        self.assertFalse(needs_business({'predicted':'inquire','error':None}))
        self.assertFalse(needs_business({'predicted':'navigate','error':'timeout'}))

    def test_consultation_does_not_require_a_business_prediction(self):
        expected={'mode':'inquire','function':'transfer'}
        self.assertTrue(route_success(expected,{'predicted':'inquire','error':None},None))
        self.assertFalse(route_success(expected,{'predicted':'navigate','error':None},
                                      {'predicted':'transfer','error':None}))

    def test_navigation_requires_both_stages_and_unknown_is_not_success(self):
        expected={'mode':'navigate','function':'transfer'}
        gate={'predicted':'navigate','error':None}
        self.assertTrue(route_success(expected,gate,{'predicted':'transfer','error':None}))
        self.assertFalse(route_success(expected,gate,{'predicted':'unknown','error':None}))
        self.assertFalse(route_success(expected,{'predicted':'inquire','error':None},None))
        self.assertFalse(route_success(expected,gate,{'predicted':'transfer','error':'invalid distribution'}))


if __name__=='__main__':
    unittest.main()
