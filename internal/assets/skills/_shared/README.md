# Shared SDD support files

This directory contains reference documents shared by the SDD skills. It is a support directory, not an invokable skill, so it intentionally has no `SKILL.md` file or skill frontmatter.

## Contents

The files in this directory define shared conventions and contracts used by multiple SDD skills, including persistence, artifact storage, phase execution, status, research, review, and skill-resolution guidance.

## Maintenance

Keep shared references in this directory as ordinary Markdown files. Install and sync flows deploy every embedded file here to each compatibility skill root. They also remove the obsolete `_shared/SKILL.md` marker while leaving this README and every shared reference in place.
