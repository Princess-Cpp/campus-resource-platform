import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('check', Path(__file__).parents[1]/'check-m3-delivery.py')
check = importlib.util.module_from_spec(spec); spec.loader.exec_module(check)

class DeliveryGateTests(unittest.TestCase):
    def green(self):
        events = [{'Action':'pass','Package':'app/internal/'+p,'Test':'Test'+p.replace('/','_')} for p in check.PACKAGES]
        events += [{'Action':'pass','Package':'app/internal/maintenance','Test':t} for t in check.REQUIRED]
        return events
    def test_real_results_required(self):
        self.assertTrue(check.summarize(self.green(),0)['passed'])
        self.assertFalse(check.summarize([],0)['passed'])
        self.assertFalse(check.summarize(self.green(),1)['passed'])
        self.assertFalse(check.summarize(self.green()+[{'Action':'fail','Package':'app/internal/maintenance'}],0)['passed'])
    def test_skip_and_missing_critical_workflow_fail(self):
        self.assertFalse(check.summarize(self.green()+[{'Action':'skip','Test':'TestSkipped'}],0)['passed'])
        self.assertFalse(check.summarize([e for e in self.green() if e['Test']!='TestNotificationRealSMTPPipeline'],0)['passed'])
        # A package with no tests isn't a passing business test or a test skip.
        self.assertTrue(check.summarize(self.green()+[{'Action':'skip','Package':'empty'}],0)['passed'])
    def test_subtests_are_counted_separately(self):
        r=check.summarize(self.green()+[{'Action':'pass','Test':'TestX/boundary'}],0)
        self.assertEqual(r['subPassed'],1)
    def test_dsn_not_echoed_and_business_database_rejected(self):
        env={k:'user:private-password@tcp(127.0.0.1:13316)/'+v+'?parseTime=true' for k,v in check.DATABASES.items()}
        self.assertEqual(check.preflight(env),[])
        env['M3_TEST_DSN']='user:private-password@tcp(127.0.0.1:13306)/campus_m3_alignment_dev?parseTime=true'
        errors=check.preflight(env)
        self.assertTrue(errors)
        self.assertNotIn('private-password',str(errors))
        self.assertTrue(check.preflight({}))
        env['M3_TEST_DSN']='user:private-password@tcp(127.0.0.1:13306)/business?x=@tcp(127.0.0.1:13316)/campus_m3_check'
        self.assertTrue(check.preflight(env))
    def test_diagnostics_redact_dsn_and_password(self):
        dsn='user:secret-value@tcp(127.0.0.1:13316)/campus_m3_check'
        scrub=check.redactor({'M3_TEST_DSN':dsn})
        text=scrub('failed '+dsn+' password secret-value')
        self.assertNotIn(dsn,text);self.assertNotIn('secret-value',text)

    def test_frontend_must_execute_nonzero_without_skips(self):
        green='# tests 30\n# pass 30\n# fail 0\n# skipped 0\n'
        self.assertTrue(check.summarize_frontend(green,0)['passed'])
        self.assertTrue(check.summarize_frontend(green.replace('#','\u2139'),0)['passed'])
        for output,code in [(green,1),('',0),(green.replace('30','0'),0),(green.replace('# skipped 0','# skipped 1'),0)]:
            self.assertFalse(check.summarize_frontend(output,code)['passed'])

if __name__=='__main__':unittest.main()
