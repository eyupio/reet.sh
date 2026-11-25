- **A machine can add to a skill only if it started that skill**: `reet agent skill import --bundle` now works only on a
  skill that machine imported itself. Another machine's skill, or one made in the browser, answers 409 and can be added
  to from the web app. This stops one machine from changing a skill that your other machines install.
