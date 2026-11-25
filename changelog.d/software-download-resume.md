- **A half-finished agent software download is continued, not started again**: when a download of OpenCode stops part-way (a dropped
  connection or a slow registry), the next attempt asks only for the rest, and the controller's mirror can finish what the
  registry started. The whole archive is still checked against its recorded checksum before anything is installed, and a file
  that fails the check is deleted, not continued. A half older than a day is not trusted.
