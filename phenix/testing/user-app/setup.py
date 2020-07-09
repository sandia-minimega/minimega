from setuptools import setup, find_packages

setup(
    name = 'phenix-app-test-user-app',
    packages = find_packages(),
    entry_points = {
        'console_scripts' : [
            'phenix-app-test-user-app = test_user_app.__main__:main'
        ]
    }
)