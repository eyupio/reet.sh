- **A database fault no longer makes a machine look revoked**: when the controller could not read its database for a
  moment, a machine was told its enrolment was missing and threw away the result of a finished update. It now gets a
  server error and sends the result again. Enrolling with a code also no longer spends the code if a database error
  stops the enrolment, and merging two machines that each have an agent answers 409 instead of 500.
