#!/usr/bin/env python3
import os, json, sys, time, subprocess
from pathlib import Path

def load_cfg():
    cfg_path = os.getenv("YUU_BRIDGE_CONFIG", str(Path.home() / ".yuuchain/bridge.json"))
    with open(cfg_path, "r") as f:
        return json.load(f), cfg_path

def check_only(cfg):
    # lazy import so --check works even if web3 is not present at import time
    from web3 import Web3
    ok = True
    print("== Check YuuChain RPC ==")
    print(f" chain_id: {cfg['yuuchain']['chain_id']}")
    print(f" rpc:      {cfg['yuuchain']['rpc']}")
    try:
        # we only need that the node answers; Web3 for EVM chains only
        import urllib.request, json as _json
        with urllib.request.urlopen(cfg['yuuchain']['rpc'] + "/status") as r:
            data = _json.loads(r.read().decode())
        net = data["result"]["node_info"]["network"]
        height = int(data["result"]["sync_info"]["latest_block_height"])
        print(f" ✅ YuuChain up: network={net} height={height}")
    except Exception as e:
        ok = False
        print(f" ❌ YuuChain status check failed: {e}")
    print("\n== Check source chains ==")
    for s in cfg.get("sources", []):
        _name = s["name"]
        rpc  = s["rpc"]
        token_addr = s["token"]["address"]
        try:
            w3 = Web3(Web3.HTTPProvider(rpc, request_kwargs={"timeout": 30}))
            latest = w3.eth.block_number
            dec = int(s["token"]["decimals"])
            print(f" [{_name}] ✅ connected, latest_block={latest}, token={token_addr}, decimals={dec}")
        except Exception as e:
            ok = False
            print(f" [{_name}] ❌ connect failed: {e}")
    return 0 if ok else 1

def main():
    if len(sys.argv) == 2 and sys.argv[1] == "--check":
        cfg, path = load_cfg()
        print(f"Using config: {path}")
        sys.exit(check_only(cfg))
    elif len(sys.argv) == 4 and sys.argv[1] == "mint-once":
        cfg, path = load_cfg()
        print(f"Using config: {path}")
        tx = sys.argv[2]
        recipient = sys.argv[3]
        sys.exit(once(cfg, tx, recipient))
    else:
        print("Usage:\n  yuu_bridge_watcher.py --check\n  yuu_bridge_watcher.py mint-once <sepolia_tx_hash> <recipient_evmos>")
        sys.exit(2)
def once(cfg, tx, recipient):
    from web3 import Web3
    import json, os

    # find sepolia source
    src = next((s for s in cfg.get("sources", []) if s.get("name")=="sepolia"), None)
    if not src:
        print("❌ sepolia missing in config."); return 2

    rpc     = src["rpc"]
    w3      = Web3(Web3.HTTPProvider(rpc, request_kwargs={"timeout": 30}))
    escrow  = Web3.to_checksum_address(src["escrow"]["address"])
    decimals = int(src["token"]["decimals"])

    # ABIs for both variants we used:
    abi_deposited_erc20 = [{
      "anonymous": False,
      "inputs": [
       {"indexed": True,  "internalType": "address", "name": "depositor", "type": "address"},
       {"indexed": False, "internalType": "uint256", "name": "amount",    "type": "uint256"},
       {"indexed": False, "internalType": "uint256", "name": "units",     "type": "uint256"}
       ],
       "name": "Deposited",
      "type": "event"
    }]
    abi_deposited_eth = [{
      "anonymous": False,
      "inputs": [
       {"indexed": True,  "internalType": "address", "name": "from",   "type": "address"},
       {"indexed": False, "internalType": "uint256", "name": "amount", "type": "uint256"}
      ],
        "name": "Deposited",
        "type": "event"
    }]

      # Standard ERC20 Transfer event
    abi_erc20_transfer = [{
    "anonymous": False,
    "inputs": [
      {"indexed": True,  "internalType": "address", "name": "from", "type": "address"},
      {"indexed": True,  "internalType": "address", "name": "to",   "type": "address"},
      {"indexed": False, "internalType": "uint256", "name": "value","type": "uint256"}
    ],
    "name": "Transfer",
    "type": "event"
  }]

    # fetch receipt
    rcpt = w3.eth.get_transaction_receipt(tx)

    # try ERC20 first
    found = None
    meta  = {}
    try:
        evt = w3.eth.contract(address=escrow, abi=abi_deposited_erc20).events.Deposited()
        for log in rcpt.logs:
            if log["address"].lower() == escrow.lower():
                try:
                    dec_ev = evt.process_log(log)
                    found  = dec_ev["args"]
                    meta["kind"] = "USDT"
                    break
                except Exception:
                    pass
    except Exception:
        pass

# fallback to ETH simple vault
    if not found:
        try:
            evt2 = w3.eth.contract(address=escrow, abi=abi_deposited_eth).events.Deposited()
            for log in rcpt.logs:
                if log["address"].lower() == escrow.lower():
                    try:
                        dec_ev = evt2.process_log(log)
                        found = dec_ev["args"]
                        meta["kind"] = "ETH"
                        break
                    except Exception:
                        pass
        except Exception:
            pass

    # Fallback: detect direct ERC20 Transfer(to=escrow) in this tx
    if not found:
        try:
            token_addr = Web3.to_checksum_address(src["token"]["address"])
            transfer_evt = w3.eth.contract(address=token_addr, abi=abi_erc20_transfer).events.Transfer()
            for log in rcpt.logs:
                if log["address"].lower() == token_addr.lower():
                    try:
                        dec_ev = transfer_evt.process_log(log)
                        args = dec_ev["args"]
                        if args["to"].lower() == escrow.lower():
                            found = {"units": int(args["value"]), "depositor": args["from"]}
                            meta["kind"] = "ERC20_TRANSFER"
                            break
                    except Exception:
                        pass
        except Exception:
            pass

    if not found:
        print("❌ No Deposited event OR ERC20 Transfer(to=escrow) found in that tx.")
        return 2
    # compute uyuu to mint
    if "units" in found:
        units = int(found["units"])          # USDT units
        if decimals <= 18:
            uyuu = units * (10 ** (18 - decimals))
        else:
            uyuu = units // (10 ** (decimals - 18))
        depositor = found["depositor"]
    else:
        # ETH case: amount is wei; 1 wei == 1 uyuu (both 18dp)
        wei = int(found["amount"])
        uyuu = wei
        depositor = found.get("from")

    meta["depositor"] = depositor
    meta_json = json.dumps(meta, separators=(",",":"))

    print("✅ Parsed deposit:")
    print(f"  depositor:    {depositor}")
    print(f"  uyuu_to_mint: {uyuu}")
    print("\nRun:")
    cmd = [
        "./build/evmosd","tx","vault","mint-from-bridge",
        "--chain-id", cfg["yuuchain"]["chain_id"],
        "--home", os.path.expanduser("~/.evmosd"),
        "--keyring-backend","os",
        "--from","bridge-operator",                      # change if your signer is different
        "--recipient", recipient,
        "--amount-uyuu", str(uyuu),
        "--source-network","sepolia",
        "--source-tx-hash", tx,
        "--source-metadata-json", meta_json,
        "--fees","10000000000000000uyuu",
        "--gas","200000",
        "-y","-o","json"
    ]
    print(" \\\n  ".join(cmd))
    return 0
def watch(cfg, recipient, confirmations=3, poll_sec=10):
    from web3 import Web3
    import json, os, time, subprocess

    # find sepolia source + escrow
    src = next((s for s in cfg.get("sources", []) if s.get("name")=="sepolia"), None)
    if not src or "escrow" not in src or "address" not in src["escrow"]:
        print("❌ sepolia/escrow missing in config."); return 2

    rpc      = src["rpc"]
    decimals = int(src["token"]["decimals"])
    w3       = Web3(Web3.HTTPProvider(rpc, request_kwargs={"timeout": 30}))
    escrow   = Web3.to_checksum_address(src["escrow"]["address"])

    # state file to remember the last processed block
    state_path = os.path.expanduser("~/.yuuchain/bridge.state.json")
    try:
        with open(state_path, "r") as f:
            state = json.load(f)
    except Exception:
        state = {}

    head        = w3.eth.block_number
    last_block  = state.get("last_block", head - confirmations)
    if last_block < 0:
        last_block = 0

    # ABIs for both vault variants we used
    abi_deposited_erc20 = [{
      "anonymous": False,
      "inputs": [
        {"indexed": True,  "internalType": "address", "name": "depositor", "type": "address"},
        {"indexed": False, "internalType": "uint256", "name": "amount",    "type": "uint256"},
        {"indexed": False, "internalType": "uint256", "name": "units",     "type": "uint256"}
      ],
      "name": "Deposited",
      "type": "event"
    }]
    abi_deposited_eth = [{
      "anonymous": False,
      "inputs": [
        {"indexed": True,  "internalType": "address", "name": "from",   "type": "address"},
        {"indexed": False, "internalType": "uint256", "name": "amount", "type": "uint256"}
      ],
      "name": "Deposited",
      "type": "event"
    }]

    evt_erc20 = w3.eth.contract(address=escrow, abi=abi_deposited_erc20).events.Deposited()
    evt_eth   = w3.eth.contract(address=escrow, abi=abi_deposited_eth).events.Deposited()

    print(f"👀 Watching escrow {escrow} from block {last_block+1} (confirmations={confirmations})")

    while True:
        try:
            head = w3.eth.block_number
            to_block = head - confirmations
            if to_block <= last_block:
                time.sleep(poll_sec); continue

            logs = w3.eth.get_logs({
                "address": escrow,
                "fromBlock": last_block + 1,
                "toBlock": to_block
            })

            for lg in logs:
                found = None
                meta  = {}

                # Try ERC20 decode
                try:
                    dec_ev = evt_erc20.process_log(lg)
                    found  = dec_ev["args"]
                    meta["kind"] = "USDT"
                except Exception:
                    # Try ETH simple vault decode
                    try:
                        dec_ev = evt_eth.process_log(lg)
                        found  = dec_ev["args"]
                        meta["kind"] = "ETH"
                    except Exception:
                        pass

                if not found:
                    continue

                # compute uyuu & metadata
                if "units" in found:
                    units = int(found["units"])
                    if decimals <= 18:
                        uyuu  = units * (10 ** (18 - decimals))
                    else:
                        uyuu  = units // (10 ** (decimals - 18))
                    depositor = found["depositor"]
                else:
                    # ETH path: wei -> uyuu 1:1
                    wei = int(found["amount"])
                    uyuu = wei
                    depositor = found.get("from")

                tx_hash = lg["transactionHash"].hex()
                meta["depositor"] = depositor
                meta_json = json.dumps(meta, separators=(",",":"))

                print(f"↳ {lg['blockNumber']} {tx_hash} :: mint {uyuu} uyuu to {recipient}")
                subprocess.run([
                    "./build/evmosd","tx","vault","mint-from-bridge",
                    "--chain-id", cfg["yuuchain"]["chain_id"],
                    "--home", os.path.expanduser("~/.evmosd"),
                    "--keyring-backend","os",
                    "--from","bridge-operator",
                    "--recipient", recipient,
                    "--amount-uyuu", str(uyuu),
                    "--source-network","sepolia",
                    "--source-tx-hash", tx_hash,
                    "--source-metadata-json", meta_json,
                    "--fees","10000000000000000uyuu",
                    "--gas","200000","-y","-o","json"
                ], check=True)

            # advance watermark and persist
            last_block = to_block
            state["last_block"] = last_block
            with open(state_path, "w") as f:
                json.dump(state, f)

            time.sleep(poll_sec)
        except KeyboardInterrupt:
            print("\nbye"); return 0
        except Exception as e:
            print("! watch error:", e)
            time.sleep(poll_sec)
def cli_main():
    import sys
    if len(sys.argv) == 2 and sys.argv[1] == "--check":
        cfg, path = load_cfg()
        print(f"Using config: {path}")
        sys.exit(check_only(cfg))
    elif len(sys.argv) == 3 and sys.argv[1] == "watch":
        cfg, path = load_cfg()
        print(f"Using config: {path}")
        recipient = sys.argv[2]
        sys.exit(watch(cfg, recipient))
    elif len(sys.argv) == 4 and sys.argv[1] == "mint-once":
        cfg, path = load_cfg()
        print(f"Using config: {path}")
        tx = sys.argv[2]
        recipient = sys.argv[3]
        sys.exit(once(cfg, tx, recipient))
    else:
        print("Usage:\n  yuu_bridge_watcher.py --check\n  yuu_bridge_watcher.py watch <recipient_evmos>\n  yuu_bridge_watcher.py mint-once <sepolia_tx_hash> <recipient_evmos>")
        sys.exit(2)

if __name__ == "__main__":
    cli_main()
