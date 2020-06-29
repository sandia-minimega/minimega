from setuptools import setup, find_packages

setup(
    name = 'phenix-scheduler-test-user-scheduler',
    packages = find_packages(),
    entry_points = {
        'console_scripts' : [
            'phenix-scheduler-test-user-scheduler = app.__main__:main'
        ]
    }
)