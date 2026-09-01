ALTER TABLE radio_stations ADD COLUMN call_sign TEXT NOT NULL DEFAULT '';
ALTER TABLE radio_stations ADD COLUMN frequency TEXT NOT NULL DEFAULT '';
ALTER TABLE radio_stations ADD COLUMN city TEXT NOT NULL DEFAULT '';
ALTER TABLE radio_stations ADD COLUMN region TEXT NOT NULL DEFAULT '';
ALTER TABLE radio_stations ADD COLUMN country TEXT NOT NULL DEFAULT 'US';
ALTER TABLE radio_stations ADD COLUMN market TEXT NOT NULL DEFAULT '';
ALTER TABLE radio_stations ADD COLUMN station_type TEXT NOT NULL DEFAULT '';
ALTER TABLE radio_stations ADD COLUMN format TEXT NOT NULL DEFAULT '';
ALTER TABLE radio_stations ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE radio_stations ADD COLUMN website_url TEXT NOT NULL DEFAULT '';

INSERT INTO radio_stations(name,call_sign,frequency,city,region,country,market,station_type,format,genre,description,stream_url,artwork)
SELECT 'WEVL 89.9 FM','WEVL','89.9 FM','Memphis','TN','US','Memphis / Mid-South','Community','Freeform','Eclectic / Community','Independent, volunteer and member-supported freeform community radio.','https://wevl.streamguys1.com/live','gradient:violet'
WHERE NOT EXISTS(SELECT 1 FROM radio_stations WHERE stream_url='https://wevl.streamguys1.com/live');
INSERT INTO radio_stations(name,call_sign,frequency,city,region,country,market,station_type,format,genre,description,stream_url,artwork)
SELECT 'WYXR 91.7 FM','WYXR','91.7 FM','Memphis','TN','US','Memphis / Mid-South','Community / Nonprofit','Freeform / Local','Eclectic / Local','Nonprofit community radio with genuinely local and freeform programming.','https://crosstown.streamguys1.com:80/live-aac','gradient:blue'
WHERE NOT EXISTS(SELECT 1 FROM radio_stations WHERE stream_url='https://crosstown.streamguys1.com:80/live-aac');
INSERT INTO radio_stations(name,call_sign,frequency,city,region,country,market,station_type,format,genre,description,stream_url,artwork)
SELECT 'WYPL 89.3 FM','WYPL','89.3 FM','Memphis','TN','US','Memphis / Mid-South','Public Library','Reading Service / Local','Spoken Word / Local','Memphis Public Library reading service and local Memphis programming.','http://ice64.securenetsystems.net/WYPL','gradient:green'
WHERE NOT EXISTS(SELECT 1 FROM radio_stations WHERE stream_url='http://ice64.securenetsystems.net/WYPL');
INSERT INTO radio_stations(name,call_sign,frequency,city,region,country,market,station_type,format,genre,description,stream_url,artwork)
SELECT 'WKNO 91.1 FM','WKNO','91.1 FM','Memphis','TN','US','Memphis / Mid-South','Public Radio','NPR / News / Talk','News / Public Radio','Locally operated NPR and public radio from Mid-South Public Communications Foundation.','https://playerservices.streamtheworld.com/api/livestream-redirect/WKNOFM_SC','gradient:red'
WHERE NOT EXISTS(SELECT 1 FROM radio_stations WHERE stream_url='https://playerservices.streamtheworld.com/api/livestream-redirect/WKNOFM_SC');
INSERT INTO radio_stations(name,call_sign,frequency,city,region,country,market,station_type,format,genre,description,stream_url,artwork)
SELECT 'KWEM-LP 93.3 FM','KWEM-LP','93.3 FM','West Memphis','AR','US','Memphis / Mid-South','Low-Power Community / College','Roots / Local','Americana / Blues / Community','Low-power community and college station continuing the historic KWEM legacy.','https://ice6.securenetsystems.net/KWEM','gradient:orange'
WHERE NOT EXISTS(SELECT 1 FROM radio_stations WHERE stream_url='https://ice6.securenetsystems.net/KWEM');
INSERT INTO radio_stations(name,call_sign,frequency,city,region,country,market,station_type,format,genre,description,stream_url,artwork)
SELECT 'WUMS 92.1 Rebel Radio','WUMS','92.1 FM','Oxford','MS','US','North Mississippi / Mid-South','College / Student','College Freeform','Alternative / College','University of Mississippi student-operated college radio.','http://130.74.34.21:8002/listen','gradient:violet'
WHERE NOT EXISTS(SELECT 1 FROM radio_stations WHERE stream_url='http://130.74.34.21:8002/listen');
INSERT INTO radio_stations(name,call_sign,frequency,city,region,country,market,station_type,format,genre,description,stream_url,artwork)
SELECT 'KASU 91.9 FM','KASU','91.9 FM','Jonesboro','AR','US','Northeast Arkansas / Mid-South','Public / University','Public Radio / Community','News / Music / Community','Arkansas State University public and community radio serving northeast Arkansas and the Mid-South.','https://kasu.streamguys1.com/live','gradient:blue'
WHERE NOT EXISTS(SELECT 1 FROM radio_stations WHERE stream_url='https://kasu.streamguys1.com/live');
