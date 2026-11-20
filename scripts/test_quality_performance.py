import unittest

from performance_diff import regression


class RegressionTest(unittest.TestCase):
    def test_consistent_slowdown(self):
        self.assertTrue(regression([1.31, 1.34, 1.30, 1.29, 1.01]))

    def test_outlier_or_small_drift(self):
        self.assertFalse(regression([1.01, 1.02, 2.5, 0.99, 1.0]))
        self.assertFalse(regression([1.14] * 5))
        self.assertFalse(regression([1.30, 1.30, 1.30, 1.00, 1.00]))
