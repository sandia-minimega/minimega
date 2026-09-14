# Releases

This project uses maintainer-managed releases through GitHub Actions.

## Release authority

- **Patch releases** may be initiated by any maintainer once CI passes and the release PR is approved.
- **Minor releases** may be initiated by any maintainer using lazy consensus:
  - Open a release PR.
  - Wait **48 business hours** for objections.
  - If there are no blocking objections and the PR is approved, the release may proceed.
- **Major releases or breaking changes** require explicit buy-in from a few active maintainers:
  - At least **two maintainer approvals** on the release PR.
  - A **1 week** objection window for other maintainers.
- **Emergency releases** may skip the waiting period when needed, such as for broken installs, a bad release, severe regressions, or actively exploited security issues.
  - Emergency releases still require approval from at least one other maintainer.

## Release process

1. Open a PR that updates the `VERSION` file.

   Use the [release PR template](https://github.com/sandia-minimega/minimega/blob/master/.github/PULL_REQUEST_TEMPLATE/release.md) when creating the PR by adding `?template=release.md` to the URL, or by selecting it from the template dropdown if available.

   Example version change:

   ```diff
   -VERSION=3.1.0
   +VERSION=3.2.0
   ```

2. Use the release PR as the place for release discussion.

   The PR should include:
   - The proposed version.
   - Whether the release is patch, minor, major, or emergency.
   - Any known compatibility or migration concerns.

3. Confirm the release requirements are met.

   Before merging the release PR:
   - CI must pass.
   - The version must be correct.
   - Required approvals must be present.
   - Any required waiting period must have elapsed.
   - Blocking objections must be resolved.

4. Merge the release PR.

5. Trigger the release workflow.

   Any maintainer may manually run the GitHub Actions workflow:

   <https://github.com/sandia-minimega/minimega/actions/workflows/release.yml>

   Use the **Run workflow** button.

6. Verify the release.

   After the workflow completes, confirm that:
   - The GitHub release was created.
   - Release artifacts were generated as expected.
   - Any downstream publishing, such as the Python package release, completed successfully.

## Notes

- Releases should be small and frequent when practical.
- The `VERSION` update PR is the official coordination point for the release.
- Because releases require a PR, at least two maintainers are involved in normal releases.
