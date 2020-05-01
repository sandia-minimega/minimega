from setuptools import setup, find_packages

setup(
    name = 'phenix-test-user-app',
    packages = find_packages(),
    entry_points = {
        'console_scripts' : [
            'phenix-test-user-app = app.__main__:main'
        ]
    }
)