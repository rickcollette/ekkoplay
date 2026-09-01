import {expect,test,type Page} from '@playwright/test'
const song={id:7,title:'Test Track',artist:'Test Artist',album:'Test Album',year:2026,format:'FLAC',duration_ms:61000,favorite:false,track_number:1,disc_number:1,codec:'flac',bitrate:900000,sample_rate:44100,channels:2,file_size:1234,original_filename:'test.flac',imported_at:'2026-08-31',file_path:'/srv/ekkoplayer/music/Rock/test.flac'}
const song2={...song,id:8,title:'Keep Selected',original_filename:'selected.flac',file_path:'/srv/ekkoplayer/music/Rock/selected.flac'}
async function mock(page:Page){await page.route('**/api/v1/**',async route=>{const u=new URL(route.request().url()),p=u.pathname,m=route.request().method();let body:unknown=[]
 if(p.endsWith('/admin/storage'))body={total_bytes:100,used_bytes:25,free_bytes:75,songs:1,albums:1,artists:1}
 else if(p.endsWith('/admin/stats'))body={songs:1,albums:1,artists:1,stations:0,missing_artwork:0,metadata_issues:0,duplicate_files:0}
 else if(p.endsWith('/player'))body={status:'paused',position_ms:0,duration_ms:0,volume:55}
 else if(p.endsWith('/admin/imports'))body=[]
 else if(p.endsWith('/admin/folders'))body=[{path:'',name:'Library root'},{path:'Rock',name:'Rock'}]
 else if(p.endsWith('/songs')&&u.searchParams.has('page'))body={items:[song],page:1,page_size:50,total:1}
 else if(p.endsWith('/songs'))body=[song,song2]
 else if(p.includes('/songs/7')&&m==='PATCH')body={...song,...route.request().postDataJSON()}
 else if(p.endsWith('/playlists'))body=[{id:9,name:'Favorites',song_count:0,updated_at:'2026-08-31'}]
 else if(p.endsWith('/radio'))body=[]
 else if(p.endsWith('/admin/backups'))body=[]
 await route.fulfill({status:m==='DELETE'?204:200,contentType:'application/json',body:m==='DELETE'?'':JSON.stringify(body)})})}
test.beforeEach(async({page})=>{await mock(page)})
test('dashboard renders live metrics',async({page})=>{await page.goto('/admin/');await expect(page.getByText('Live library, playback and storage status.')).toBeVisible();await expect(page.getByText('1',{exact:true}).first()).toBeVisible()})
test('folder manager shows files only inside their actual folder',async({page})=>{await page.goto('/admin/folders');await expect(page.getByRole('button',{name:'Open folder Rock'})).toBeVisible();await expect(page.getByText('test.flac',{exact:true})).not.toBeVisible();await page.getByRole('button',{name:'Open folder Rock'}).click();await expect(page.getByText('test.flac',{exact:true})).toBeVisible();await page.getByRole('button',{name:'Up',exact:true}).click();await expect(page.getByText('test.flac',{exact:true})).not.toBeVisible();page.once('dialog',dialog=>dialog.accept('Jazz'));const request=page.waitForRequest(r=>r.url().endsWith('/admin/folders')&&r.method()==='POST');await page.getByRole('button',{name:/New folder/}).click();expect((await request).postDataJSON()).toEqual({path:'Jazz'})})
test('folder playlist action respects deselection after select all',async({page})=>{await page.goto('/admin/folders');await page.getByRole('button',{name:'Open folder Rock'}).click();await page.getByRole('button',{name:'Select all'}).click();await page.getByLabel('Select file test.flac').uncheck();const request=page.waitForRequest(r=>r.url().includes('/playlists/9/songs')&&r.method()==='POST');await page.getByLabel('Add current folder to playlist').selectOption('9');expect((await request).postDataJSON()).toEqual({song_ids:[8]})})
test('playlist and radio empty states expose creation actions',async({page})=>{await page.goto('/admin/playlists');await expect(page.getByRole('button',{name:/New playlist/})).toBeVisible();await page.goto('/admin/radio');await page.getByRole('button',{name:/Add station/}).click();await expect(page.getByRole('heading',{name:'New station'})).toBeVisible()})
test('imports exposes upload and scan workflows',async({page})=>{await page.goto('/admin/imports');await expect(page.getByText('Drop music here')).toBeVisible();await expect(page.getByRole('button',{name:/Scan folder/})).toBeVisible()})
