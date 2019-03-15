// Copyright 2018 The go-committee Authors
// This file is part of the go-committee library.
//
// The go-committee library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-committee library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-committee library. If not, see <http://www.gnu.org/licenses/>.

package node

import (
	"time"
	"sync"
	"github.com/usechain/go-usechain/cmd/utils"
	"github.com/usechain/go-usechain/log"
	"github.com/usechain/go-usechain/common"
	"github.com/usechain/go-committee/console"
	"github.com/usechain/go-committee/account"
	"github.com/usechain/go-committee/contract/manager"
	"github.com/usechain/go-committee/shamirkey"
	"github.com/usechain/go-committee/shamirkey/core"
	"github.com/usechain/go-committee/node/config"
	//"github.com/usechain/go-committee/contract/creditTesting"
	"github.com/usechain/go-committee/contract/creditNew"
	"fmt"
)

var (
	globalConfig  config.Usechain
	cache		  *core.SharePool
	keypool		  *core.KeyPool
	wg			  sync.WaitGroup
)

// init the committee global config
func Initial() {
	log.Info("Committee node initializing ......")
	time.Sleep(time.Second * 5)

	//init config
	config.Init(&globalConfig)

	//init the share pool
	cache = core.NewSharePool()
	keypool = core.NewKeyPool()

	//Check the committee account format && legality
	addr := globalConfig.UserProfile.Address
	if addr == "" || !common.IsHexAddress(addr) {
		utils.Fatalf("Please fill in correct committee address in conf")
	} else {
		globalConfig.Kstore = account.DefaultKeystore()
		signer, err := account.CommitteeAccount(common.HexToAddress(addr), globalConfig.Kstore)
		if err != nil {
			utils.Fatalf("Please import committee corresponding keystore file")
		}

		log.Warn("Please unlock the committee account")
		log.Warn("Enter \"committee.unlock \"passwd\"\"")
		fmt.Print("Please input password >>> ")
		select {
		case passwd := <- account.CommitteePasswd:
			err = globalConfig.Kstore.TimedUnlock(signer, passwd, 0)
			if err != nil {
				utils.Fatalf("Unlock the committee account failed %v", err)
			}
		}
	}
	log.Info("Usechain Committee Console Initialization Complete")
	return
}

//committee work main process
func run() {
	// Listening the network msg
	go func(){
		shamirkey.ShamirKeySharesListening(globalConfig.UserProfile, cache, keypool)
	}()

	// Process handle
	for {
		globalConfig.Workstat = config.GetState(globalConfig)
		log.Debug("The process is in stage", "workStat", globalConfig.Workstat)

		switch globalConfig.Workstat {
		case config.NotCommittee:
			utils.Fatalf("Not a legal committee address!")

		case config.Selected:
			log.Debug("selected, please confirm")
			//Get committe id from contract
			id, err := manager.GetSelfCommitteeID(globalConfig)
			if err != nil || id == -1{
				log.Error("Get CommitteeID failed", "err", err)
			}
			globalConfig.UserProfile.CommitteeID = id
			globalConfig.UserProfile.Role = "Sharer"
			config.UpdateProfile(globalConfig.UserProfile)

			//Confirm & upload self asym key
			manager.ConfirmAndKeyUpload(globalConfig)

		case config.WaittingOther:
			log.Debug("Just waitting!")

		case config.KeyGenerating:
			log.Warn("KeyGenerating")
			//Read from contract to update certid, upload asym key, and download all committee certID and asym key
			shamirkey.InitShamirCommitteeNumber(globalConfig)

			//Check whether get enough shares
			go func(){
				wg.Add(1)
				defer wg.Done()
				keypool.ShamirKeyShareCheck(&globalConfig)
			}()

			//Request private share & self part generation
			shamirkey.ShamirKeySharesGenerate(globalConfig.UserProfile.CommitteeID, keypool)
			shamirkey.SendRequestShares(globalConfig.UserProfile.CommitteeID)
			wg.Wait()

		case config.Verifying:
			log.Debug("Verifying...")

			//Read from contract to update certid, upload asym key, and download all committee certID and asym key
			shamirkey.InitShamirCommitteeNumber(globalConfig)

			// Verifying
			go func(){
				shamirkey.AccountVerifyProcess(&globalConfig, cache)
			}()

			creditNew.ScanCreditSystemAccount(&globalConfig, cache, core.CommitteeNodeList, core.CommitteeMax)

		default:
			utils.Fatalf("Unknown state")
		}
		time.Sleep(time.Second * 30)
	}
	return
}

//entry for committee working process
func Start() {
	Initial()
	con := console.New(globalConfig)

	run()
	// Start console service
	con.Start()

}

