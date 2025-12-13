# YuuChain (evmos-yuuchain)

Minimal repo for the YuuChain (Evmos/Cosmos) work.

Bridge v1 (Sepolia) shows:
1) User deposits on Sepolia → watcher parses event
2) Vault mint-from-bridge mints uyuu on YuuChain (local node)

---

## Quick Start (Bridge v1)

### 1) Python environment (repo; persistent)
```bash
python3 -m venv .venv && . .venv/bin/activate
pip install web3
```
