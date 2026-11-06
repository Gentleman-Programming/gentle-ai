"""Guest-only predicate proof; never import the full privileged-layout journey."""
import ast
import pathlib
import re
import types
import unittest
import yaml

SOURCE = pathlib.Path(__file__).with_name('shell-linux-user-install-guest.py')
TREE = ast.parse(SOURCE.read_text())
CHECKS = [node.args[0] for node in ast.walk(TREE)
          if isinstance(node, ast.Call) and isinstance(node.func, ast.Name)
          and node.func.id == 'require' and len(node.args) == 2
          and isinstance(node.args[1], ast.Constant)
          and node.args[1].value == 'pnpm root identity/integrity differs']
assert len(CHECKS) == 1, 'ambiguous live root receipt check'
PREDICATE = compile(ast.Expression(CHECKS[0]), str(SOURCE), 'eval')
HELPERS = [node for node in TREE.body if isinstance(node, ast.FunctionDef)
           and node.name == 'pnpm_root_receipt']
GLOBALS = {'re': re, 'yaml': yaml}
if HELPERS:
    assert len(HELPERS) == 1
    exec(compile(ast.Module(body=HELPERS, type_ignores=[]), str(SOURCE), 'exec'), GLOBALS)


class ReceiptContract(unittest.TestCase):
    name = '@scope/package'
    version = '1.0.0'
    sri = 'sha512-' + 'A' * 86 + '=='

    def lock(self, package=None, version=None, sri=None, importer=None):
        package, version, sri = package or self.name, version or self.version, sri or self.sri
        return (f"lockfileVersion: '9.0'\nimporters:\n  .:\n    dependencies:\n"
                f"      '{self.name}':\n        specifier: {self.version}\n"
                f"        version: {importer or self.version}\npackages:\n"
                f"  '{package}@{version}':\n    resolution: {{integrity: {sri}}}\n"
                "snapshots: {}\n")

    def accepted(self, text):
        location = types.SimpleNamespace(is_relative_to=lambda _: True,
                                         stat=lambda: types.SimpleNamespace(st_uid=1002))
        return eval(PREDICATE, GLOBALS, {'location': location, 'home': object(),
                    'item': {'version': self.version}, 'name': self.name,
                    'expected': self.version, 'sri': self.sri, 'text': text})

    def test_actual_package_and_importer_role(self):
        self.assertTrue(self.accepted(self.lock()))
        self.assertTrue(self.accepted(self.lock(importer='1.0.0(peer@2.0.0)')))
        for prefix in ['^', '~']:
            self.assertTrue(self.accepted(self.lock().replace('specifier: 1.0.0', 'specifier: ' + prefix + '1.0.0')))

    def test_dead_literal_cannot_supply_root_integrity(self):
        for changes in [{'package': 'other'}, {'version': '2.0.0'},
                        {'sri': 'sha512-' + 'B' * 86 + '=='}, {'importer': '2.0.0'}]:
            with self.subTest(changes=changes):
                self.assertFalse(self.accepted(self.lock(**changes) + f'# unused: {self.sri}\n'))

    def test_missing_and_ambiguous_roles_refuse(self):
        for text in [f'# unused: {self.sri}\n',
                     self.lock().replace("    resolution:", "    unused:"),
                     self.lock() + self.lock(),
                     self.lock() + 'packages: {}\n',
                     self.lock() + 'packages : {}\n',
                     self.lock() + '"pack\\u0061ges": {}\n',
                     self.lock() + '? packages\n: {}\n',
                     self.lock().replace('importers:\n', 'unused: &receipt {}\nimporters:\n') + 'packages: *receipt\n',
                     self.lock().replace('snapshots: {}', '  \'@scope/package@1.0.0\': {}\nsnapshots: {}'),
                     self.lock().replace('packages:\n', '      \'@scope/package\': {}\npackages:\n')]:
            with self.subTest(text=text[:40]):
                self.assertFalse(self.accepted(text))


if __name__ == '__main__':
    unittest.main()
