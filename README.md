go-readconfig
=============

Abstraction to handle configuration management which reads automatically on updates

Tries to determine automatically where to place and create user configuration.
If no user environment can be determined, return the system configuration.

The package also uses inotify to monitor file changes, and returns the new reads through a channel.
