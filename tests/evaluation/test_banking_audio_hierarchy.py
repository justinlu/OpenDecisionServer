import unittest

from banking_audio_hierarchy import specification, menu, domain_for, business_correct, route_correct, oracle_menu


class HierarchyTests(unittest.TestCase):
    def test_domains_partition_leaves_and_reduce_candidate_count(self):
        spec=specification()
        leaves=[leaf for group in spec['domains'].values() for leaf in group]
        self.assertEqual(len(leaves),len(set(leaves)))
        self.assertEqual(len(leaves),15)
        self.assertEqual(set(leaves),set(spec['business']['criteria'])-{'unknown'})
        self.assertEqual(len(spec['domain']['criteria']),6)
        self.assertTrue(all(3<=len(menu(spec,domain)['criteria'])<=6 for domain in spec['domains']))
        self.assertIsNone(menu(spec,'unknown'))

    def test_wrong_domain_and_unknown_cannot_count_as_correct_business(self):
        spec=specification()
        answer=lambda predicted:dict(predicted=predicted,error=None)
        self.assertFalse(business_correct(spec,'withdraw',answer('account'),answer('withdraw')))
        self.assertFalse(business_correct(spec,'withdraw',answer('unknown'),None))
        self.assertTrue(business_correct(spec,'withdraw',answer('cash'),answer('withdraw')))

    def test_consultation_misgate_stays_wrong_even_if_leaf_is_correct(self):
        spec=specification()
        answer=lambda predicted:dict(predicted=predicted,error=None)
        expected=dict(mode='inquire',function='withdraw')
        self.assertFalse(route_correct(spec,expected,answer('navigate'),answer('cash'),answer('withdraw')))
        self.assertTrue(route_correct(spec,expected,answer('inquire'),None,None))

    def test_oracle_menus_are_nested_diagnostics_with_same_descriptions(self):
        spec=specification()
        small=oracle_menu(spec,'withdraw',3,'fixed')
        medium=oracle_menu(spec,'withdraw',5,'fixed')
        self.assertEqual(len(small['criteria']),3)
        self.assertEqual(len(medium['criteria']),5)
        self.assertLessEqual(set(small['criteria']),set(medium['criteria']))
        self.assertTrue({'withdraw','unknown'}<=set(small['criteria']))
        for key,value in medium['criteria'].items():self.assertEqual(value,spec['business']['criteria'][key])
        self.assertEqual(domain_for(spec,'loss'),'card')
        self.assertEqual(domain_for(spec,'credit_repay'),'credit')


if __name__=='__main__':unittest.main()
