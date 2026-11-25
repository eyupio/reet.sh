- **A skill whose script has run is no longer treated as edited**: running a bundled Python script writes a
  `__pycache__` folder inside the skill, which made reet back up the copy, put the original back and report it as
  edited every time. Those files are now ignored when reet checks a copy.
