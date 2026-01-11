#!/usr/bin/env python3
import os, json, sys, time, subprocess
from pathlib import Path
from typing import Optional
from decimal import Decimal, getcontext, ROUND_FLOOR

# high precision for safe integer conversion (avoid floats)
getcontext().prec = 80

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

        with urllib.request.urlopen(cfg["yuuchain"]["rpc"] + "/status") as r:
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
        rpc = s["rpc"]
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


def _tm_node_from_cfg(cfg) -> Optional[str]:
    """
    Return a Tendermint RPC URL suitable for `evmosd --node`.
    Your cfg["yuuchain"]["rpc"] is often http(s)://...; evmosd commonly uses tcp://...
    """
    node = cfg.get("yuuchain", {}).get("rpc")
    if not node:
        return None
    node = str(node).strip()
    if node.startswith("http://"):
        return "tcp://" + node[len("http://"):]
    if node.startswith("https://"):
        return "tcp://" + node[len("https://"):]
    return node

def _parse_last_json_object(noisy_text: str) -> Optional[dict]:
    """
    evmosd may print module fmt.Println() lines before the JSON.
    Extract the LAST JSON object from the output.
    """
    if not noisy_text:
        return None
    text = noisy_text.strip()
    for i in range(len(text) - 1, -1, -1):
        if text[i] != "{":
            continue
        cand = text[i:]
        try:
            obj = __import__("json").loads(cand)
            if isinstance(obj, dict):
                return obj
        except Exception:
            continue
    return None

def _get_vault_price_usdt_per_yuu(cfg) -> Optional[Decimal]:
    """
    Fetch current YUU price from YuuChain (USDT per 1 YUU), as Decimal.
    Uses: evmosd query vault price -o json
    Handles noisy stdout lines printed before JSON.
    """
    node = _tm_node_from_cfg(cfg)
    if not node:
        return None
    cmd = ["./build/evmosd", "query", "vault", "price", "--node", node, "-o", "json"]
    try:
        import subprocess
        out = subprocess.check_output(cmd, text=True, stderr=subprocess.STDOUT)
        j = _parse_last_json_object(out)
        if not isinstance(j, dict):
            return None
        for k in ("price", "Price"):
            if k in j and str(j[k]).strip():
                return Decimal(str(j[k]).strip())
        if "result" in j and isinstance(j["result"], dict):
            r = j["result"]
            for k in ("price", "Price"):
                if k in r and str(r[k]).strip():
                    return Decimal(str(r[k]).strip())
    except Exception:
        return None
    return None

def _uyuu_from_uusdt_and_price(uusdt_units: int, price_usdt_per_yuu: Decimal) -> int:
    """
    Convert deposit amount (uusdt, 6dp) -> mint amount (uyuu, 18dp)
    using the on-chain price (USDT per 1 YUU).

      uyuu = floor( uusdt * 1e18 / (price * 1e6) )
    """
    if price_usdt_per_yuu <= 0:
        raise ValueError("price must be positive")
    units = Decimal(int(uusdt_units))
    num = units * (Decimal(10) ** 18)
    den = price_usdt_per_yuu * (Decimal(10) ** 6)
    uyuu = (num / den).to_integral_value(rounding=ROUND_FLOOR)
    return int(uyuu)

def _uusdt_from_source_units(units_raw: int, token_decimals: int) -> int:
    """
    Normalize a source token amount into 'uusdt' units (6 decimals).
    """
    u = int(units_raw)
    d = int(token_decimals)
    if d == 6:
        return u
    if d > 6:
        return u // (10 ** (d - 6))
    return u * (10 ** (6 - d))


def _get_source(cfg, name: Optional[str]):
    sources = cfg.get("sources", [])
    if not sources:
        return None
    if not name:
        return sources[0]
    return next((s for s in sources if s.get("name") == name), None)
def once(cfg, tx, recipient, source_name=None):
    from web3 import Web3

    src = _get_source(cfg, source_name)
    if not src:
        print(f"❌ source '{source_name}' missing in config.")
        return 2

    src_name = src.get("name", "unknown")

    rpc = src["rpc"]
    w3 = Web3(Web3.HTTPProvider(rpc, request_kwargs={"timeout": 30}))
    escrow = Web3.to_checksum_address(src["escrow"]["address"])
    decimals = int(src["token"]["decimals"])

    # ABIs for both variants we used:
    # NEW (recommended): Deposited(depositor, amount, units, recipient, depositId)
    abi_deposited_erc20_v2 = [
        {
            "anonymous": False,
            "inputs": [
                {"indexed": True, "internalType": "address", "name": "depositor", "type": "address"},
                {"indexed": False, "internalType": "uint256", "name": "amount", "type": "uint256"},
                {"indexed": False, "internalType": "uint256", "name": "units", "type": "uint256"},
                {"indexed": False, "internalType": "string", "name": "recipient", "type": "string"},
                {"indexed": False, "internalType": "uint256", "name": "depositId", "type": "uint256"},
            ],
            "name": "Deposited",
            "type": "event",
        }
    ]

    # OLD: Deposited(depositor, amount, units)
    abi_deposited_erc20 = [
        {
            "anonymous": False,
            "inputs": [
                {"indexed": True, "internalType": "address", "name": "depositor", "type": "address"},
                {"indexed": False, "internalType": "uint256", "name": "amount", "type": "uint256"},
                {"indexed": False, "internalType": "uint256", "name": "units", "type": "uint256"},
            ],
            "name": "Deposited",
            "type": "event",
        }
    ]

    # ETH simple vault: Deposited(from, amount)
    abi_deposited_eth = [
        {
            "anonymous": False,
            "inputs": [
                {"indexed": True, "internalType": "address", "name": "from", "type": "address"},
                {"indexed": False, "internalType": "uint256", "name": "amount", "type": "uint256"},
            ],
            "name": "Deposited",
            "type": "event",
        }
    ]

    # Standard ERC20 Transfer event
    abi_erc20_transfer = [
        {
            "anonymous": False,
            "inputs": [
                {"indexed": True, "internalType": "address", "name": "from", "type": "address"},
                {"indexed": True, "internalType": "address", "name": "to", "type": "address"},
                {"indexed": False, "internalType": "uint256", "name": "value", "type": "uint256"},
            ],
            "name": "Transfer",
            "type": "event",
        }
    ]

    # fetch receipt
    rcpt = w3.eth.get_transaction_receipt(tx)

    found = None
    meta = {}
    found_log_index = None

    # Try escrow Deposited events (v2 then v1)
    try:
        evt_v2 = w3.eth.contract(address=escrow, abi=abi_deposited_erc20_v2).events.Deposited()
        evt_v1 = w3.eth.contract(address=escrow, abi=abi_deposited_erc20).events.Deposited()

        for log in rcpt.logs:
            if log["address"].lower() != escrow.lower():
                continue

            try:
                dec_ev = evt_v2.process_log(log)
                found = dec_ev["args"]
                found_log_index = int(log.get("logIndex", 0))
                meta["kind"] = "USDT_V2"
                break
            except Exception:
                pass

            try:
                dec_ev = evt_v1.process_log(log)
                found = dec_ev["args"]
                found_log_index = int(log.get("logIndex", 0))
                meta["kind"] = "USDT"
                break
            except Exception:
                pass
    except Exception:
        pass

    # fallback to ETH simple vault
    if not found:
        try:
            evt_eth = w3.eth.contract(address=escrow, abi=abi_deposited_eth).events.Deposited()
            for log in rcpt.logs:
                if log["address"].lower() != escrow.lower():
                    continue
                try:
                    dec_ev = evt_eth.process_log(log)
                    found = dec_ev["args"]
                    found_log_index = int(log.get("logIndex", 0))
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
                if log["address"].lower() != token_addr.lower():
                    continue
                try:
                    dec_ev = transfer_evt.process_log(log)
                    args = dec_ev["args"]
                    if args["to"].lower() == escrow.lower():
                        found = {"units": int(args["value"]), "depositor": args["from"]}
                        found_log_index = int(log.get("logIndex", 0))
                        meta["kind"] = "ERC20_TRANSFER"
                        break
                except Exception:
                    pass
        except Exception:
            pass

    if not found:
        print("❌ No Deposited event OR ERC20 Transfer(to=escrow) found in that tx.")
        return 2

    if found_log_index is None:
        found_log_index = 0

    # compute uyuu to mint
    uusdt_units = 0
    if "units" in found:
        units_raw = int(found["units"])  # token native units
        uusdt_units = _uusdt_from_source_units(units_raw, decimals)
        price = _get_vault_price_usdt_per_yuu(cfg)
        if price is None:
            raise RuntimeError("could not query vault price (evmosd query vault price)")
        uyuu = _uyuu_from_uusdt_and_price(uusdt_units, price)
        depositor = found["depositor"]
    else:
        # ETH case: amount is wei; 1 wei == 1 uyuu (both 18dp)
        uyuu = int(found["amount"])
        depositor = found.get("from")

    # Prefer recipient from event (v2) if present
    ev_recipient = found.get("recipient")
    if ev_recipient:
        recipient = ev_recipient

    # Idempotent source id: txhash:logIndex
    source_id = f"{tx}:{found_log_index}"

    # enrich metadata
    meta["depositor"] = depositor
    meta["recipient"] = recipient
    meta["escrow"] = escrow
    meta["token_decimals"] = decimals
    meta["tx_hash"] = tx
    meta["log_index"] = found_log_index
    if "depositId" in found:
        meta["deposit_id"] = int(found["depositId"])
    if "amount" in found:
        meta["amount_raw"] = str(found["amount"])
    if "units" in found:
        meta["units_raw"] = str(found["units"])

    meta_json = json.dumps(meta, separators=(",", ":"))

    token_symbol = src.get("token", {}).get("symbol", "USDT")
    token_address = src.get("token", {}).get("address", "")
    source_amount_uusdt = str(uusdt_units)

    print("✅ Parsed deposit:")
    print(f"  depositor:    {depositor}")
    print(f"  uyuu_to_mint: {uyuu}")
    print(f"  recipient:    {recipient}")
    print(f"  source_id:    {source_id}")
    print("\nRun:")

    cmd = [
        "./build/evmosd",
        "tx",
        "vault",
        "mint-from-bridge",
        "--chain-id",
        cfg["yuuchain"]["chain_id"],
        "--home",
        os.path.expanduser("~/.evmosd"),
        "--keyring-backend",
        "os",
        "--from",
        "bridge-operator",  # change if your signer is different
        "--recipient",
        recipient,
        "--amount-uyuu",
        str(uyuu),
        "--source-network",
        src_name,
        "--source-tx-hash",
        source_id,
        "--source-amount-uusdt",
        source_amount_uusdt,
        "--token-symbol",
        token_symbol,
        "--token-address",
        token_address,
        "--fees",
        "10000000000000000uyuu",
        "--gas",
        "200000",
        "-y",
        "-o",
        "json",
    ]
    print(" \\\n  ".join(cmd))
    return 0


def watch(cfg, recipient, source_name=None, confirmations=3, poll_sec=10):
    from web3 import Web3

    src = _get_source(cfg, source_name)
    if not src or "escrow" not in src or "address" not in src["escrow"]:
        print(f"❌ source '{source_name}' / escrow missing in config.")
        return 2

    src_name = src.get("name", "unknown")

    rpc = src["rpc"]
    decimals = int(src["token"]["decimals"])
    w3 = Web3(Web3.HTTPProvider(rpc, request_kwargs={"timeout": 30}))
    escrow = Web3.to_checksum_address(src["escrow"]["address"])

    # Prefer confirmations from config if present
    confirmations = int(src.get("min_confirmations", confirmations))

    # state file to remember the last processed block
    evm_chain_id = src.get("evm_chain_id", "unknown")
    state_path = os.path.expanduser(f"~/.yuuchain/bridge.state.{src_name}.{evm_chain_id}.json")
    try:
        with open(state_path, "r") as f:
            state = json.load(f)
    except Exception:
        state = {}

    head = w3.eth.block_number
    last_block = state.get("last_block", head - confirmations)
    if last_block < 0:
        last_block = 0

    # ABIs
    abi_deposited_erc20_v2 = [
        {
            "anonymous": False,
            "inputs": [
                {"indexed": True, "internalType": "address", "name": "depositor", "type": "address"},
                {"indexed": False, "internalType": "uint256", "name": "amount", "type": "uint256"},
                {"indexed": False, "internalType": "uint256", "name": "units", "type": "uint256"},
                {"indexed": False, "internalType": "string", "name": "recipient", "type": "string"},
                {"indexed": False, "internalType": "uint256", "name": "depositId", "type": "uint256"},
            ],
            "name": "Deposited",
            "type": "event",
        }
    ]

    abi_deposited_erc20 = [
        {
            "anonymous": False,
            "inputs": [
                {"indexed": True, "internalType": "address", "name": "depositor", "type": "address"},
                {"indexed": False, "internalType": "uint256", "name": "amount", "type": "uint256"},
                {"indexed": False, "internalType": "uint256", "name": "units", "type": "uint256"},
            ],
            "name": "Deposited",
            "type": "event",
        }
    ]

    abi_deposited_eth = [
        {
            "anonymous": False,
            "inputs": [
                {"indexed": True, "internalType": "address", "name": "from", "type": "address"},
                {"indexed": False, "internalType": "uint256", "name": "amount", "type": "uint256"},
            ],
            "name": "Deposited",
            "type": "event",
        }
    ]

    evt_erc20_v2 = w3.eth.contract(address=escrow, abi=abi_deposited_erc20_v2).events.Deposited()
    evt_erc20_v1 = w3.eth.contract(address=escrow, abi=abi_deposited_erc20).events.Deposited()
    evt_eth = w3.eth.contract(address=escrow, abi=abi_deposited_eth).events.Deposited()

    print(f"👀 Watching [{src_name}] escrow {escrow} from block {last_block+1} (confirmations={confirmations})")

    while True:
        try:
            head = w3.eth.block_number
            # fetch vault price once per loop (for ERC20 deposits)
            price = _get_vault_price_usdt_per_yuu(cfg)
            if price is None:
                raise RuntimeError("could not query vault price (evmosd query vault price)")

            to_block = head - confirmations
            if to_block <= last_block:
                time.sleep(poll_sec)
                continue

            logs = w3.eth.get_logs(
                {
                    "address": escrow,
                    "fromBlock": last_block + 1,
                    "toBlock": to_block,
                }
            )

            for lg in logs:
                found = None
                meta = {}
                uusdt_units = 0

                # Try ERC20 v2 decode (preferred)
                try:
                    dec_ev = evt_erc20_v2.process_log(lg)
                    found = dec_ev["args"]
                    meta["kind"] = "USDT_V2"
                except Exception:
                    # Try ERC20 v1 decode
                    try:
                        dec_ev = evt_erc20_v1.process_log(lg)
                        found = dec_ev["args"]
                        meta["kind"] = "USDT"
                    except Exception:
                        # Try ETH simple vault decode
                        try:
                            dec_ev = evt_eth.process_log(lg)
                            found = dec_ev["args"]
                            meta["kind"] = "ETH"
                        except Exception:
                            pass

                if not found:
                    continue

                # compute uyuu & depositor
                if "units" in found:
                      units_raw = int(found["units"])
                      uusdt_units = _uusdt_from_source_units(units_raw, decimals)
                      uyuu = _uyuu_from_uusdt_and_price(uusdt_units, price)
                      depositor = found["depositor"]
                else:
                    uyuu = int(found["amount"])
                    depositor = found.get("from")

                # recipient: v2 overrides CLI arg
                ev_recipient = found.get("recipient")
                mint_recipient = ev_recipient if ev_recipient else recipient

                tx_hash = lg["transactionHash"].hex()
                log_index = int(lg.get("logIndex", 0))
                source_id = f"{tx_hash}:{log_index}"

                meta["depositor"] = depositor
                meta["recipient"] = mint_recipient
                meta["escrow"] = escrow
                meta["token_decimals"] = decimals
                meta["tx_hash"] = tx_hash
                meta["log_index"] = log_index
                if "depositId" in found:
                    meta["deposit_id"] = int(found["depositId"])
                if "amount" in found:
                    meta["amount_raw"] = str(found["amount"])
                if "units" in found:
                    meta["units_raw"] = str(found["units"])

                meta_json = json.dumps(meta, separators=(",", ":"))

                print(f"↳ {lg['blockNumber']} {source_id} :: mint {uyuu} uyuu to {mint_recipient}")

                # WATCH-ONLY mode: don't broadcast any tx, only print what would happen.
                if os.getenv("YUU_WATCH_ONLY", "").lower() in ("1", "true", "yes"):
                    print("[WATCH_ONLY] skipping mint tx broadcast")
                    continue

                token_symbol = src.get("token", {}).get("symbol", "USDT")
                token_address = src.get("token", {}).get("address", "")
                source_amount_uusdt = str(uusdt_units)

                subprocess.run(
                    [
                        "./build/evmosd",
                        "tx",
                        "vault",
                        "mint-from-bridge",
                        "--chain-id",
                        cfg["yuuchain"]["chain_id"],
                        "--home",
                        os.path.expanduser("~/.evmosd"),
                        "--keyring-backend",
                        "os",
                        "--from",
                        "bridge-operator",
                        "--recipient",
                        mint_recipient,
                        "--amount-uyuu",
                        str(uyuu),
                        "--source-network",
                        src_name,
                        "--source-tx-hash",
                        source_id,
                        "--source-amount-uusdt",
                          source_amount_uusdt,
                          "--token-symbol",
                          token_symbol,
                          "--token-address",
                          token_address,
                        "--fees",
                        "10000000000000000uyuu",
                        "--gas",
                        "200000",
                        "-y",
                        "-o",
                        "json",
                    ],
                    check=True,
                )

            # advance watermark and persist
            last_block = to_block
            state["last_block"] = last_block
            with open(state_path, "w") as f:
                json.dump(state, f)

            time.sleep(poll_sec)

        except KeyboardInterrupt:
            print("\nbye")
            return 0
        except Exception as e:
            print("! watch error:", e)
            time.sleep(poll_sec)

def cli_main():
    if len(sys.argv) == 2 and sys.argv[1] == "--check":
        cfg, path = load_cfg()
        print(f"Using config: {path}")
        sys.exit(check_only(cfg))

    # NEW: watch <source_name> <recipient_evmos>
    elif len(sys.argv) == 4 and sys.argv[1] == "watch":
        cfg, path = load_cfg()
        print(f"Using config: {path}")
        source_name = sys.argv[2]
        recipient = sys.argv[3]
        sys.exit(watch(cfg, recipient, source_name=source_name))

    # BACKCOMPAT: watch <recipient_evmos> (uses first source)
    elif len(sys.argv) == 3 and sys.argv[1] == "watch":
        cfg, path = load_cfg()
        print(f"Using config: {path}")
        recipient = sys.argv[2]
        src0 = cfg.get("sources", [{}])[0].get("name")
        print(f"[WARN] No source_name provided; defaulting to first source: {src0}")
        sys.exit(watch(cfg, recipient, source_name=src0))

    # NEW: mint-once <source_name> <tx_hash> <recipient_evmos>
    elif len(sys.argv) == 5 and sys.argv[1] == "mint-once":
        cfg, path = load_cfg()
        print(f"Using config: {path}")
        source_name = sys.argv[2]
        tx = sys.argv[3]
        recipient = sys.argv[4]
        sys.exit(once(cfg, tx, recipient, source_name=source_name))

    # BACKCOMPAT: mint-once <tx_hash> <recipient_evmos> (uses first source)
    elif len(sys.argv) == 4 and sys.argv[1] == "mint-once":
        cfg, path = load_cfg()
        print(f"Using config: {path}")
        tx = sys.argv[2]
        recipient = sys.argv[3]
        src0 = cfg.get("sources", [{}])[0].get("name")
        print(f"[WARN] No source_name provided; defaulting to first source: {src0}")
        sys.exit(once(cfg, tx, recipient, source_name=src0))

    else:
        print(
            "Usage:\n"
            "  yuu_bridge_watcher.py --check\n"
            "  yuu_bridge_watcher.py watch <source_name> <recipient_evmos>\n"
            "  yuu_bridge_watcher.py mint-once <source_name> <tx_hash> <recipient_evmos>\n"
            "\nBackcompat:\n"
            "  yuu_bridge_watcher.py watch <recipient_evmos>               (uses first source)\n"
            "  yuu_bridge_watcher.py mint-once <tx_hash> <recipient_evmos> (uses first source)"
        )
        sys.exit(2)


if __name__ == "__main__":
    cli_main()
