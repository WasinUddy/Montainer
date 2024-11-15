import sys
import re
import os
from typing import Tuple


def parse_changelog(changelog_path: str = 'CHANGELOG.md', with_v: bool = False) -> Tuple[str, str, str]:
    with open(changelog_path) as f:
        changelog = f.read()

    # Get the first (Highest Latest) version
    version_pattern = r'## \[(v?\d+\.\d+\.\d+)\] - (\d{4}-\d{2}-\d{2})'
    match = re.search(version_pattern, changelog)
    if not match:
        print('Version not found in CHANGELOG.md', file=sys.stderr)
        sys.exit(1)

    version, date = match.groups()
    version = version.strip()
    date = date.strip()

    # Extract Latest version details
    latest_version_pattern = r'## \[v?{0}\] - {1}(.*?)(?:\n## \[v?\d+\.\d+\.\d+\] - \d{{4}}-\d{{2}}-\d{{2}}|\Z)'.format(
        re.escape(version), re.escape(date))
    match = re.search(latest_version_pattern, changelog, re.DOTALL)
    if not match:
        print('Latest version details not found in CHANGELOG.md', file=sys.stderr)
        sys.exit(1)

    latest_version_details = match.group(1).strip()

    return version, date, latest_version_details


if __name__ == '__main__':
    # Parse sys.argv if specified
    version, date, latest_version_details = parse_changelog(*sys.argv[1:])

    os.makedirs('github_action_outputs', exist_ok=True)

    # Write to file
    with open('./github_action_outputs/version.txt', 'w') as f:
        f.write(version)

    with open('./github_action_outputs/date.txt', 'w') as f:
        f.write(date)

    with open('./github_action_outputs/latest_version_details.txt', 'w') as f:
        f.write(latest_version_details)

    print(f'Version: {version}\nDate: {date}\nLatest Version Details:\n{latest_version_details}')