Thank you for your interest in contributing to minimega! We welcome contributions from everyone and appreciate your efforts to improve our project. This guide will help you understand how to contribute effectively.

> Note: A failure to follow this guide may result in delay of PRs until there is compliance. 

## Table of Contents

- [Getting Started](#getting-started)
- [How to Contribute](#how-to-contribute)
  - [Reporting Issues](#reporting-issues)
  - [Suggesting Enhancements](#suggesting-enhancements)
  - [Submitting Code](#submitting-code)
- [License](#license)

## Getting Started

1. **Fork the Repository**: Click the "Fork" button at the top right of the repository page to create your own copy of the project.

2. **Clone Your Fork**: Clone your forked repository to your local machine using:
   ```bash
   git clone https://github.com/<your-username>/minimega.git
   ```

3. **Set Upstream Remote**: Add the original repository as an upstream remote to keep your fork up to date:
    ```bash
    git remote add upstream https://github.com/sandia-minimega/minimega.git
    ```

4. **(Optional) Update Your Fork with Upstream**: To keep your fork up to date with the original repository, follow these steps:
    * Fetch the latest changes from the upstream repository
        ```bash
        git fetch upstream
        ```
    * Merge the changes from the upstream master branch into your local master branch
        ```bash
        git checkout master
        git merge upstream/master
        ```
    * Push the updated master branch to your forked repository
        ```bash
        git push origin master
        ```

## How to Contribute

### Reporting Issues

If you encounter a bug or have a feature request, please open an issue in the [Issues](https://github.com/sandia-minimega/minimega/issues) section. Be sure to include:

- A clear description of the issue.
- Steps to reproduce the issue.
- Any relevant screenshots or logs.

### Suggesting Enhancements

We welcome suggestions for improvements! Please open an issue to discuss your ideas before implementing them. 

### Submitting Code

1. **Create a Branch**: Create a new branch (on your fork of the repository) for your feature or bug fix using [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) notation. The branch name should follow this format:
    ```bash
    type/description
    ```
    Where `type` can be one of the following:
    - `feat`: A new feature
    - `fix`: A bug fix
    - `docs`: Documentation only changes
    - `style`: Changes that do not affect the meaning of the code (white-space, formatting, etc.)
    - `refactor`: A code change that neither fixes a bug nor adds a feature
    - `test`: Adding missing tests or correcting existing tests
    - `chore`: Changes to the build process or auxiliary tools and libraries

    Example:
    ```bash
    git checkout -b feat/add-user-authentication
    ```

2. **Make Your Changes**: Implement your changes.

3. **Stage Your Changes**: Use the `git add` command to stage the changes you want to commit. You can stage specific files or all changes:
    To stage specific files:
    ```bash
    git add path/to/your/file1 path/to/your/file2
    ```

    To stage all changes:
    ```bash
    git add .
    ```

3. **Commit Your Changes**: Commit your changes using [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) notation. Your commit message should follow this format:
    ```bash
    type(scope): subject
    ```
    * **Scope** is optional and can be used to indicate the area of the codebase affected by the change.
    * **Subject** should be a short description of the change.

    Example:
    ```bash
    git commit -m "feat(auth): add user authentication feature"
    ```
    If you need to write a longer commit message, you can do so by running `git commit`. This will open your default text editor where you can write a detailed commit message. The first line should be a brief summary conforming to the format above, followed by a blank line, and then a more detailed explanation. 

    Example:
    ```bash
    feat(auth): add user authentication feature

    This commit introduces a new authentication system that allows users to log in using their email and password. It also includes validation for user input and error handling.
    ```

4. **Push to Your Fork**: Push to your branch and ensure CI/CD workflows are passing.
    
    Example:
    ```bash
    git push origin feat/add-user-authentication
    ```
    
5. **Open a Pull Request**: Go to the original repository and open a [pull request](https://github.com/sandia-minimega/minimega/pulls). Provide a clear description of your changes and reference any related issues.

## License
By contributing to this project, you agree that your contributions will be licensed under the [GNU General Public License v3.0](https://github.com/sandia-minimega/minimega/blob/master/LICENSE) License.
