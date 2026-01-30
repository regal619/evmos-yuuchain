#!/usr/bin/env python3
import os, json, sys, time, subprocess, hashlib
from pathlib import Path
from typing import Optional, Dict, Any, List
from decimal import Decimal, getcontext, ROUND_FLOOR

# high precision for safe integer conversion (avoid floats)
getcontext().prec = 80


def load_cfg():
    cfg_path = os.getenv("YUU_BRIDGE_CONFIG", str(Path.home() / ".yuuchain/bridge.json"))
    with open(cfg_path, "r") as f:
        return json.load(f), cfg_path


def _tm_node_from_cfg(cfg) -> Optional[str]:
    """
    Return a Tendermint RPC URL suitable for `evmosd --node`.
    cfg["yuuchain"]["rpc"] might be http(s)://...; evmosd commonly uses tcp://...
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
    evmosd may print fmt.Println() lines before the JSON.
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
    Fetch current YUU issuance price from YuuChain (USDT per 1 YUU), as Decimal.
    Uses: evmosd query vault price -o json
    Handles noisy stdout lines printed before JSON.
    """
    node = _tm_node_from_cfg(cfg)
    if not node:
        return None
    cmd = ["./build/evmosd", "query", "vault", "price", "--node", node, "-o", "json"]
    try:
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


def _get_source(cfg, name: Optional[str]):
    sources = cfg.get("sources", [])
    if not sources:
        return None
    if not name:
        return sources[0]
    return next((s for s in sources if s.get("name") == name), None)


def _load_state(path: str) -> Dict[str, Any]:
    try:
        with open(path, "r") as f:
            return json.load(f)
    except Exception:
        return {}


def _save_state(path: str, state: Dict[str, Any]) -> None:
    os.makedirs(os.path.dirname(path), exist_ok=True)
    tmp = path + ".tmp"
    with open(tmp, "w") as f:
        json.dump(state, f)
    os.replace(tmp, path)


def _http_get_json(url: str, headers: Optional[Dict[str, str]] = None, timeout: int = 30) -> Dict[str, Any]:
    import urllib.request

    req = urllib.request.Request(url, headers=headers or {})
    with urllib.request.urlopen(req, timeout=timeout) as r:
        raw = r.read().decode("utf-8")
    return json.loads(raw)

def _http_post_json(url: str, body: Dict[str, Any], headers: Optional[Dict[str, str]] = None, timeout: int = 30) -> Dict[str, Any]:
    import urllib.request

    data = json.dumps(body).encode("utf-8")
    h = {"Content-Type": "application/json"}
    if headers:
        h.update(headers)

    req = urllib.request.Request(url, data=data, headers=h, method="POST")
    with urllib.request.urlopen(req, timeout=timeout) as r:
        raw = r.read().decode("utf-8")
    return json.loads(raw)


def _tron_tx_block_number(trongrid_base: str, headers: Dict[str, str], txid: str) -> int:
    base = trongrid_base.rstrip("/")

    # Try both endpoints (some TronGrid setups expose one or the other)
    for path in ("/wallet/gettransactioninfobyid", "/walletsolidity/gettransactioninfobyid"):
        try:
            j = _http_post_json(base + path, {"value": txid}, headers=headers)
            bn = j.get("blockNumber") or j.get("block_number")
            if bn is not None:
                return int(bn)
        except Exception:
            continue

    return 0

def _tron_gettransactionbyid(trongrid_base: str, headers: Dict[str, str], txid: str) -> Dict[str, Any]:
    """
    Fetch raw TRON transaction so we can read call data:
      raw_data.contract[0].parameter.value.data
    """
    base = trongrid_base.rstrip("/")
    return _http_post_json(base + "/wallet/gettransactionbyid", {"value": txid, "visible": True}, headers=headers)


def _abi_read_word(hexstr: str, offset: int) -> int:
    """Read a 32-byte ABI word (64 hex chars) as int from hexstr at hex offset."""
    return int(hexstr[offset:offset + 64], 16)


def _abi_read_bytes(hexstr: str, start_bytes: int) -> bytes:
    """Read dynamic bytes/string located at byte-offset start_bytes in ABI payload."""
    start = start_bytes * 2
    strlen = _abi_read_word(hexstr, start)
    data_start = start + 64
    data_hex = hexstr[data_start:data_start + strlen * 2]
    return bytes.fromhex(data_hex)


def _extract_evmos_from_calldata(calldata_hex: str) -> Optional[str]:
    """
    Tries to decode recipient evmos address from ABI-encoded call data.
    Supports common patterns:
      deposit(uint256,string)
      deposit(string,uint256)
      deposit(string)
    """
    if not calldata_hex:
        return None
    h = calldata_hex.strip().lower()
    if h.startswith("0x"):
        h = h[2:]
    if len(h) < 8:
        return None

    # remove 4-byte selector
    payload = h[8:]

    def looks_like_evmos(s: str) -> bool:
        s = s.strip()
        return s.startswith("evmos1") and len(s) >= 20

    # Case A: (uint256,string) -> word1 is offset
    try:
        off = _abi_read_word(payload, 64)
        b = _abi_read_bytes(payload, off)
        s = b.decode("utf-8", errors="ignore").strip("\x00").strip()
        if looks_like_evmos(s):
            return s
    except Exception:
        pass

    # Case B: (string,uint256) -> word0 is offset
    try:
        off = _abi_read_word(payload, 0)
        b = _abi_read_bytes(payload, off)
        s = b.decode("utf-8", errors="ignore").strip("\x00").strip()
        if looks_like_evmos(s):
            return s
    except Exception:
        pass

    # Case C: (string) -> word0 is offset
    try:
        off = _abi_read_word(payload, 0)
        b = _abi_read_bytes(payload, off)
        s = b.decode("utf-8", errors="ignore").strip("\x00").strip()
        if looks_like_evmos(s):
            return s
    except Exception:
        pass

    return None


def _tron_tx_recipient_evmos(trongrid_base: str, headers: Dict[str, str], txid: str) -> Optional[str]:
    """
    Pull tx call data and decode recipient evmos address from it (if present).
    """
    tx = _tron_gettransactionbyid(trongrid_base, headers, txid)
    raw = tx.get("raw_data") or {}
    contracts = raw.get("contract") or []
    if not contracts:
        return None
    param = (contracts[0].get("parameter") or {}).get("value") or {}
    data = param.get("data")
    if not isinstance(data, str) or not data:
        return None
    return _extract_evmos_from_calldata(data)

def _tron_head_block(trongrid_base: str, headers: Dict[str, str]) -> int:
    base = trongrid_base.rstrip("/")

    # Try TronGrid v1 endpoint first
    url1 = base + "/v1/blocks/latest"
    try:
        j = _http_get_json(url1, headers=headers)
        data = j.get("data") or []
        if data:
            num = data[0]["block_header"]["raw_data"]["number"]
            return int(num)
    except Exception:
        pass

    # Fallback: fullnode-style endpoint (works on many TronGrid deployments)
    url2 = base + "/wallet/getnowblock"
    j2 = _http_get_json(url2, headers=headers)
    num2 = j2["block_header"]["raw_data"]["number"]
    return int(num2)

def _tron_trc20_incoming(
    trongrid_base: str,
    headers: Dict[str, str],
    escrow_addr: str,
    usdt_contract: str,
    min_timestamp_ms: int,
    limit: int = 200,
) -> List[Dict[str, Any]]:
    """
    Fetch TRC20 transfers involving escrow address.
    We filter to incoming transfers (to == escrow) and contract == usdt_contract.
    Endpoint:
      /v1/accounts/{address}/transactions/trc20
    """
    base = trongrid_base.rstrip("/")
    url = (
        f"{base}/v1/accounts/{escrow_addr}/transactions/trc20"
        f"?only_confirmed=true&limit={limit}"
        f"&min_timestamp={min_timestamp_ms}"
    )
    j = _http_get_json(url, headers=headers)
    items = j.get("data") or []
    out = []
    for it in items:
        try:
            to_addr = it.get("to")
            if not to_addr:
                continue
            if to_addr != escrow_addr:
                continue
            token_info = it.get("token_info") or {}
            contract = token_info.get("address") or ""
            if contract != usdt_contract:
                continue
            out.append(it)
        except Exception:
            continue
    # sort by time ascending so we process older first
    out.sort(key=lambda x: int(x.get("block_timestamp", 0)))
    return out


def check_only(cfg):
    # Basic check: can read YuuChain status + can reach TronGrid latest block
    ok = True
    print("== Check YuuChain RPC ==")
    print(f" chain_id: {cfg['yuuchain']['chain_id']}")
    print(f" rpc:      {cfg['yuuchain']['rpc']}")
    try:
        import urllib.request, json as _json
        with urllib.request.urlopen(cfg["yuuchain"]["rpc"] + "/status") as r:
            data = _json.loads(r.read().decode())
        net = data["result"]["node_info"]["network"]
        height = int(data["result"]["sync_info"]["latest_block_height"])
        print(f" ✅ YuuChain up: network={net} height={height}")
    except Exception as e:
        ok = False
        print(f" ❌ YuuChain status check failed: {e}")

    print("\n== Check TRON source ==")
    src = _get_source(cfg, "tron") or _get_source(cfg, None)
    if not src:
        ok = False
        print(" ❌ No sources[] found in config")
    else:
        try:
            trongrid = src.get("trongrid") or "https://api.trongrid.io"
            api_key = src.get("trongrid_api_key", "").strip()
            headers = {}
            if api_key:
                headers["TRON-PRO-API-KEY"] = api_key
            head = _tron_head_block(trongrid, headers)
            escrow = (src.get("escrow") or {}).get("address")
            usdt = (src.get("token") or {}).get("address")
            print(f" ✅ TronGrid reachable: head_block={head}")
            print(f"    escrow={escrow}")
            print(f"    usdt_trc20={usdt}")
        except Exception as e:
            ok = False
            print(f" ❌ TronGrid check failed: {e}")

    return 0 if ok else 1


def watch_tron(cfg, recipient_evmos: str, source_name: str = "tron", confirmations: int = 20, poll_sec: int = 12):
    src = _get_source(cfg, source_name)
    if not src:
        print(f"❌ source '{source_name}' missing in config.")
        return 2

    src_name = src.get("name", source_name)
    escrow_addr = (src.get("escrow") or {}).get("address")
    if not escrow_addr:
        print("❌ config missing sources[].escrow.address (Tron address starting with T...)")
        return 2

    token = src.get("token") or {}
    token_symbol = token.get("symbol", "USDT")
    token_decimals = int(token.get("decimals", 6))
    usdt_contract = token.get("address")
    if not usdt_contract:
        print("❌ config missing sources[].token.address (USDT TRC20 contract address)")
        return 2

    # confirmations can be overridden in config
    confirmations = int(src.get("min_confirmations", confirmations))

    trongrid = src.get("trongrid") or "https://api.trongrid.io"
    api_key = str(src.get("trongrid_api_key", "")).strip()
    headers: Dict[str, str] = {}
    if api_key:
        headers["TRON-PRO-API-KEY"] = api_key

    # State: watermark by timestamp (ms) + processed ids
    chain_tag = str(src.get("tron_chain", "mainnet"))
    state_path = os.path.expanduser(f"~/.yuuchain/bridge.state.{src_name}.{chain_tag}.json")
    state = _load_state(state_path)

    # start timestamp: last_ts or now-5min
    last_ts = int(state.get("last_ts_ms", int(time.time() * 1000) - 5 * 60 * 1000))

    processed = state.get("processed_source_ids", [])
    if not isinstance(processed, list):
        processed = []
    processed_set = set(str(x) for x in processed)

    print(f"👀 Watching [TRON:{src_name}] escrow {escrow_addr}")
    print(f"    USDT(TRC20) {usdt_contract} decimals={token_decimals}")
    print(f"    confirmations={confirmations} poll_sec={poll_sec}")
    print(f"    state={state_path}")

    while True:
        try:
            head = _tron_head_block(trongrid, headers)

            # fetch new incoming transfers
            transfers = _tron_trc20_incoming(
                trongrid_base=trongrid,
                headers=headers,
                escrow_addr=escrow_addr,
                usdt_contract=usdt_contract,
                min_timestamp_ms=last_ts,
                limit=200,
            )

            # process each confirmed-enough transfer
            for it in transfers:
                txid = it.get("transaction_id") or ""
                from_addr = it.get("from") or ""
                to_addr = it.get("to") or ""
                value_raw = it.get("value") or "0"

                # TronGrid TRC20 list sometimes returns block_number=None.
                # If missing, fetch it from gettransactioninfobyid.
                bn_raw = it.get("block_number")
                try:
                    block_number = int(bn_raw) if bn_raw is not None else 0
                except Exception:
                    block_number = 0

                if block_number <= 0 and txid:
                    block_number = _tron_tx_block_number(trongrid, headers, txid)

                ts_ms = int(it.get("block_timestamp", 0))

                # confirmations check
                if block_number <= 0:
                    continue
                if (head - block_number) < confirmations:
                    continue

                # normalize to uusdt (6dp)
                # TronGrid returns integer string in token base units
                units_raw = int(value_raw)

                if token_decimals != 6:
                    # normalize into uusdt units (6dp)
                    if token_decimals > 6:
                        uusdt_units = units_raw // (10 ** (token_decimals - 6))
                    else:
                        uusdt_units = units_raw * (10 ** (6 - token_decimals))
                else:
                    uusdt_units = units_raw

                # deterministic idempotency key
                source_id = txid

                # TRON deposits MUST carry the recipient in the escrow tx calldata.
                # Never fallback to the CLI arg (prevents silent wrong mints).
                mint_to = None
                try:
                    mint_to = _tron_tx_recipient_evmos(trongrid, headers, txid)
                except Exception as e:
                    print(f"[WARN] Could not decode recipient from TRON tx {txid}: {e}")

                if not mint_to:
                    print(f"[SKIP] TRON tx {txid} has no decodable recipient; not minting.")
                    last_ts = max(last_ts, ts_ms + 1)
                    continue

                if source_id in processed_set:
                    # already minted
                    last_ts = max(last_ts, ts_ms + 1)
                    continue

                # compute uyuu to mint
                price = _get_vault_price_usdt_per_yuu(cfg)
                if price is None:
                    raise RuntimeError("could not query vault price (evmosd query vault price)")

                uyuu = _uyuu_from_uusdt_and_price(uusdt_units, price)

                print(f"↳ TRON block={block_number} head={head} {source_id} :: mint {uyuu} uyuu to {mint_to}")
                print(f"    from={from_addr} value={value_raw} (uusdt={uusdt_units}) price={price}")

                if os.getenv("YUU_WATCH_ONLY", "").lower() in ("1", "true", "yes"):
                    print("[WATCH_ONLY] skipping mint tx broadcast")
                else:
                      # Feed keyring passphrase to evmosd (non-interactive for systemd)
                      passphrase = os.getenv("YUU_KEYRING_PASS", "")
                      stdin_data = (passphrase + "\n") if passphrase else None

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
                          "bridge-operator",
                          "--recipient",
                          mint_to,
                          "--amount-uyuu",
                          str(uyuu),
                          "--source-network",
                          "tron",
                          "--source-tx-hash",
                          source_id,
                          "--source-amount-uusdt",
                          str(uusdt_units),
                          "--token-symbol",
                          token_symbol,
                          "--token-address",
                          usdt_contract,
                          "--fees",
                          "10000000000000000uyuu",
                          "--gas",
                          "200000",
                          "-y",
                          "-o",
                          "json",
                      ]
                      out = subprocess.check_output(
                          cmd,
                          input=stdin_data,
                          text=True,
                          stderr=subprocess.STDOUT,
                      )
                      print(out.strip())

                # mark processed + persist
                processed_set.add(source_id)
                processed.append(source_id)

                # keep list bounded
                if len(processed) > 5000:
                    processed = processed[-3000:]
                    processed_set = set(processed)

                last_ts = max(last_ts, ts_ms + 1)
                state["last_ts_ms"] = last_ts
                state["processed_source_ids"] = processed
                _save_state(state_path, state)

            # advance watermark even if no transfers (prevents re-reading same slice forever)
            state["last_ts_ms"] = last_ts
            state["processed_source_ids"] = processed
            _save_state(state_path, state)

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

    # watch tron <recipient_evmos>
    elif len(sys.argv) == 4 and sys.argv[1] == "watch":
        cfg, path = load_cfg()
        print(f"Using config: {path}")
        source_name = sys.argv[2]
        recipient = sys.argv[3]
        if source_name != "tron":
            print("[WARN] This watcher is for TRON. Use: watch tron <recipient>")
        sys.exit(watch_tron(cfg, recipient_evmos=recipient, source_name="tron"))

    else:
        print(
            "Usage:\n"
            "  yuu_bridge_watcher_tron.py --check\n"
            "  yuu_bridge_watcher_tron.py watch tron <recipient_evmos>\n"
            "\nEnv:\n"
            "  YUU_BRIDGE_CONFIG=./configs/bridge.tron-mainnet.json\n"
            "  YUU_WATCH_ONLY=true   (prints but does not broadcast)\n"
        )
        sys.exit(2)


if __name__ == "__main__":
    cli_main()
