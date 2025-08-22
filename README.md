# scrape

Log file needs to be created manually and assigned the correct permissions.


## Location

Create the target dir and log file here:

`/var/log/scrape/scrape.log`

## Permissions

Change the ownership of the log directory (and log file) to the user that will execute the binary.  For exmaple:

`chown -R locaUser:localGroup /var/log/scrape`

The above will give ownership of the `/var/log/scrape` directory (and recursively to all files within it), to locaUser 
(and localGroup).  This is required to allow scrape to open and write to the log file.

Note:  In the above case the binary must be executed by the `localUser`.