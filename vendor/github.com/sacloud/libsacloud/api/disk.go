package api

import (
	"fmt"
	"github.com/sacloud/libsacloud/sacloud"
	"time"
)

var (
	// allowDiskEditTags ディスクの編集可否判定に用いるタグ
	allowDiskEditTags = []string{
		"os-unix",
		"os-linux",
	}
)

// DiskAPI ディスクAPI
type DiskAPI struct {
	*baseAPI
}

// NewDiskAPI ディスクAPI作成
func NewDiskAPI(client *Client) *DiskAPI {
	return &DiskAPI{
		&baseAPI{
			client: client,
			// FuncGetResourceURL
			FuncGetResourceURL: func() string {
				return "disk"
			},
		},
	}
}

// SortByConnectionOrder 接続順でのソート
func (api *DiskAPI) SortByConnectionOrder(reverse bool) *DiskAPI {
	api.sortBy("ConnectionOrder", reverse)
	return api
}

// WithServerID サーバーID条件
func (api *DiskAPI) WithServerID(id int64) *DiskAPI {
	api.FilterBy("Server.ID", id)
	return api
}

// Create 新規作成
func (api *DiskAPI) Create(value *sacloud.Disk) (*sacloud.Disk, error) {
	//HACK: さくらのAPI側仕様: 戻り値:Successがbool値へ変換できないため文字列で受ける
	type diskResponse struct {
		*sacloud.Response
		// Success
		Success string `json:",omitempty"`
	}
	res := &diskResponse{}
	err := api.create(api.createRequest(value), res)
	if err != nil {
		return nil, err
	}
	return res.Disk, nil
}

// NewCondig ディスクの修正用パラメーター作成
func (api *DiskAPI) NewCondig() *sacloud.DiskEditValue {
	return &sacloud.DiskEditValue{}
}

// Config ディスクの修正
func (api *DiskAPI) Config(id int64, disk *sacloud.DiskEditValue) (bool, error) {
	var (
		method = "PUT"
		uri    = fmt.Sprintf("%s/%d/config", api.getResourceURL(), id)
	)

	return api.modify(method, uri, disk)
}

func (api *DiskAPI) install(id int64, body *sacloud.Disk) (bool, error) {
	var (
		method = "PUT"
		uri    = fmt.Sprintf("%s/%d/install", api.getResourceURL(), id)
	)
	//HACK: さくらのAPI側仕様: 戻り値:Successがbool値へ変換できないため文字列で受ける
	type diskResponse struct {
		*sacloud.ResultFlagValue
		// Success
		Success string `json:",omitempty"`
	}
	res := &diskResponse{}
	err := api.baseAPI.request(method, uri, body, res)
	if err != nil {
		return false, err
	}
	return res.IsOk, nil
}

// ReinstallFromBlank ブランクディスクから再インストール
func (api *DiskAPI) ReinstallFromBlank(id int64, sizeMB int) (bool, error) {
	var body = &sacloud.Disk{}
	body.SetSizeMB(sizeMB)

	return api.install(id, body)
}

// ReinstallFromArchive アーカイブからの再インストール
func (api *DiskAPI) ReinstallFromArchive(id int64, archiveID int64, distantFrom ...int64) (bool, error) {
	var body = &sacloud.Disk{}
	body.SetSourceArchive(archiveID)
	if len(distantFrom) > 0 {
		body.SetDistantFrom(distantFrom)
	}
	return api.install(id, body)
}

// ReinstallFromDisk ディスクからの再インストール
func (api *DiskAPI) ReinstallFromDisk(id int64, diskID int64, distantFrom ...int64) (bool, error) {
	var body = &sacloud.Disk{}
	body.SetSourceDisk(diskID)
	if len(distantFrom) > 0 {
		body.SetDistantFrom(distantFrom)
	}
	return api.install(id, body)
}

// ToBlank ディスクを空にする
func (api *DiskAPI) ToBlank(diskID int64) (bool, error) {
	var (
		method = "PUT"
		uri    = fmt.Sprintf("%s/%d/to/blank", api.getResourceURL(), diskID)
	)
	return api.modify(method, uri, nil)
}

// ResizePartition パーティションのリサイズ
func (api *DiskAPI) ResizePartition(diskID int64) (bool, error) {
	var (
		method = "PUT"
		uri    = fmt.Sprintf("%s/%d/resize-partition", api.getResourceURL(), diskID)
	)
	return api.modify(method, uri, nil)
}

// DisconnectFromServer サーバーとの接続解除
func (api *DiskAPI) DisconnectFromServer(diskID int64) (bool, error) {
	var (
		method = "DELETE"
		uri    = fmt.Sprintf("%s/%d/to/server", api.getResourceURL(), diskID)
	)
	return api.modify(method, uri, nil)
}

// ConnectToServer サーバーとの接続
func (api *DiskAPI) ConnectToServer(diskID int64, serverID int64) (bool, error) {
	var (
		method = "PUT"
		uri    = fmt.Sprintf("%s/%d/to/server/%d", api.getResourceURL(), diskID, serverID)
	)
	return api.modify(method, uri, nil)
}

// State ディスクの状態を取得し有効な状態か判定
func (api *DiskAPI) State(diskID int64) (bool, error) {
	disk, err := api.Read(diskID)
	if err != nil {
		return false, err
	}
	return disk.IsAvailable(), nil
}

// SleepWhileCopying コピー終了まで待機
func (api *DiskAPI) SleepWhileCopying(diskID int64, timeout time.Duration) error {
	current := 0 * time.Second
	interval := 5 * time.Second
	for {
		available, err := api.State(diskID)
		if err != nil {
			return err
		}

		if available {
			return nil
		}
		time.Sleep(interval)
		current += interval

		if timeout > 0 && current > timeout {
			return fmt.Errorf("Timeout: WaitforAvailable")
		}
	}
}

// AsyncSleepWhileCopying コピー終了まで待機(非同期)
func (api *DiskAPI) AsyncSleepWhileCopying(id int64, timeout time.Duration) (chan (*sacloud.Disk), chan (*sacloud.Disk), chan (error)) {
	complete := make(chan *sacloud.Disk)
	progress := make(chan *sacloud.Disk)
	err := make(chan error)

	go func() {
		for {
			select {
			case <-time.After(5 * time.Second):
				disk, e := api.Read(id)
				if e != nil {
					err <- e
					return
				}

				progress <- disk

				if disk.IsAvailable() {
					complete <- disk
					return
				}
				if disk.IsFailed() {
					err <- fmt.Errorf("Failed: Create disk is failed: %#v", disk)
					return
				}

			case <-time.After(timeout):
				err <- fmt.Errorf("Timeout: AsyncSleepWhileCopying[ID:%d]", id)
				return
			}
		}
	}()
	return complete, progress, err
}

// Monitor アクティビティーモニター取得
func (api *DiskAPI) Monitor(id int64, body *sacloud.ResourceMonitorRequest) (*sacloud.MonitorValues, error) {
	return api.baseAPI.monitor(id, body)
}

// CanEditDisk ディスクの修正が可能か判定
func (api *DiskAPI) CanEditDisk(id int64) (bool, error) {

	disk, err := api.Read(id)
	if err != nil {
		return false, err
	}

	if disk == nil {
		return false, nil
	}

	// BundleInfoがあれば編集不可
	if disk.BundleInfo != nil {
		// Windows
		return false, nil
	}

	// ソースアーカイブ/ソースディスクともに持っていない場合
	if disk.SourceArchive == nil && disk.SourceDisk == nil {
		//ブランクディスクがソース
		return false, nil
	}

	for _, t := range allowDiskEditTags {
		if disk.HasTag(t) {
			// 対応OSインストール済みディスク
			return true, nil
		}
	}

	// ここまできても判定できないならソースに投げる
	if disk.SourceDisk != nil && disk.SourceDisk.Availability != "discontinued" {
		return api.client.Disk.CanEditDisk(disk.SourceDisk.ID)
	}
	if disk.SourceArchive != nil && disk.SourceArchive.Availability != "discontinued" {
		return api.client.Archive.CanEditDisk(disk.SourceArchive.ID)
	}

	return false, nil

}
