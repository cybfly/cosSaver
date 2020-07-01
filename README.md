# cosSaver
Used to upload your dir(including subdirs and files) into TencentCloud COS

## What does this program used to 
You can use this program to upload your files to COS,
It will visit your directory and upload all the sub directorys and files and upload them.
You can skip some directory by setting the config skippedDir.
Once the program finished, it will write the uploaded time(lastUploadTime), and the files failed to upload(uploadFailedFiles).
Next time you run the program it will skip files which are before this time.
And you can also choose to upload the failed files by set the config "action" to "FAILED".

## Sample configuration
{
	"action":"ALL",
	"secretID":"YOUR_ID",
	"sercretKey":"YOUR_KEY",
	"bucketURL":"https://XXX.cos.ap-guangzhou.myqcloud.com",
	"defaultUploadTime":"1987-06-21 01:02:03",
	"sourceDir":"D:\\Photos",
	"initKeyDir":"",
	"skippedDir":["/2015","/2018","/2017","/APRIL"]
}
