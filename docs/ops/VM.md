# The momoshtrem VM

> Spec · 2026-10-15 · #42 · Epic #32 · Plan: [`PLAN.md`](PLAN.md)
> The VM that replaces `momoshtrem-vm` ([`AS-IS.md`](AS-IS.md)). It is created by hand in Proxmox from this page: PLAN §1 rules out IaC for the VM. Ansible (#34) takes over from the state in §5.

This page fixes three of the old VM's problems from the start:
- its root LV took only half the volume group and filled up;
- its data disk was mounted by `/dev/sdb`;
- it had no guest agent (AS-IS §1, §2).

Docker, log caps, the firewall and the boot unit belong to Ansible (#34, [`HOST.md`](HOST.md)), not to this page.

If the VM changes (RAM, a disk size, the cache cap), update this page in the same commit.

## 1. Spec

**Proxmox hardware**

| Setting | Value | Why |
|---|---|---|
| Name | `momoshtrem` | It's also the hostname. Any free VM ID will do. |
| Machine | q35, BIOS OVMF (UEFI), with an EFI disk and pre-enrolled keys | Ubuntu boots with Secure Boot. No TPM is needed. |
| CPU | 1 socket, 4 cores, type `host` | The same as today. With one host and no migration, `host` exposes every CPU feature. |
| RAM | 8 GiB; leave the ballooning device at its default | The same as today. Use less during the overlap if the host is short (§2). |
| SCSI controller | VirtIO SCSI single | Gives each disk its own IO thread. |
| Root disk | `scsi0`, 32 GiB, `discard=on`, `iothread=1`, `ssd=1` | See the sizing notes below. |
| Data disk | `scsi1`, 120 GiB, `discard=on`, `iothread=1`, `ssd=1` | Holds `/mnt/data`. See the sizing notes below. |
| NIC | VirtIO (virtio-net) on `vmbr0`, no VLAN | |
| QEMU guest agent | on, with guest-trim after a disk move | #34 installs the package. Until then Proxmox shows the agent as not running. |
| Start at boot | on | A Proxmox host reboot brings the stack back. |

Set `ssd=1` only if the storage is an SSD.

`discard` lets the guest's weekly `fstrim` return freed blocks to a thin pool. This matters on the data disk, because the torrent cache keeps deleting pieces.

**Guest**

| Setting | Value | Why |
|---|---|---|
| OS | Ubuntu 24.04 LTS server, the latest 24.04.x live-server ISO, not minimized | |
| Encryption | none | The VM has to boot unattended. |
| Root disk layout | LVM; `ubuntu-lv` fills `ubuntu-vg` | The installer takes only half by default (§4). |
| Data disk layout | one GPT partition, ext4, mounted at `/mnt/data` by UUID | The installer writes a UUID entry in fstab. |
| Network | DHCP | The IP comes from a router reservation for the VM's MAC. It keeps the address it first got, 192.168.0.99, for good; at cutover the router forwards move to it (§6). |
| User | `agha`, in `sudo`, with a password | Ansible asks for this sudo password at run time (PLAN §7). Keep it in the password manager. |
| SSH | OpenSSH, key-only, with the Mac's key; root login off | |
| Snaps | none | Docker comes from Docker's apt repository (#34), and the Docker snap would clash with it. |

**Root disk sizing.**
- The root disk holds the OS, the pulled images and capped logs.
- The VM never builds (D7), and it keeps only the current and previous images (PLAN §8), which come to about 2–4 GB.
- The old 32 GiB disk filled up because of builds on the box, build cache and uncapped logs. None of these happen here.

**Data disk sizing.**
- The torrent cache is the only thing on `/mnt/data` that grows to fill the space. Its cap is `torrent.global_cache_size` in `config.yaml`, in MiB. Today it's 92160, which is 90 GiB.
- Everything else on the disk is small today: Postgres 67 MB, subtitles 208 KB and Jackett 396 KB (AS-IS §9).
- **Rule: keep the cap at or below 75% of the data disk.** The rest covers ext4 overhead and reserved blocks, the database and subtitles as they grow, and any local dumps #35 keeps.
- With today's cap that gives 120 GiB, the same as the old disk, which ran at 81% used.
- If either the cap or the disk size changes, change the other one too.
- **Built at 70 GiB.** The VM was created with a 70 GiB data disk, which is grown to 120 GiB after cutover (§6). Until then the cap is 52 GiB (`global_cache_size: 53760`).

## 2. Before you create it: can the host run both VMs?

The old VM keeps running until cutover, and it stays on the host, stopped, until the new one has three green nightly backups (#42 §4). Check the host first, from the Proxmox host shell:

```bash
free -g                # RAM: total and available
qm list                # the VMs and their memory
qm config <old-vmid>   # the old VM's memory and disks
pvesm status           # storage: type, total and available
lvs                    # if the storage is LVM-thin: the thin pool's Data%
```

- **RAM.** Take the host's total, subtract about 2 GiB for Proxmox, and subtract the old VM's 8 GiB. The new VM gets what's left, up to 8 GiB.
  - If less than 8 GiB is left, build with what fits, but no less than 4 GiB.
  - Raise it to 8 GiB at cutover, once the old VM is shut down (§6).
- **Snapshots.** The storage must support them: `lvmthin`, `zfspool`, or qcow2 on a directory. Plain `lvm` doesn't. Snapshots are the way back at cutover (PLAN §6).
- **Space.** On thin storage the new VM uses little at first, because the torrent cache isn't copied (#42 §3).
  - After cutover the cache refills towards its cap, while the old VM still holds its own 90 GB.
  - If the pool can't hold both caches, lower `global_cache_size` on the new VM until the old one is deleted.
  - Keep a thin pool's `Data%` under about 90%. A full thin pool can corrupt every VM on it.

Record the result on #42.

## 3. Create the VM in Proxmox

First, get the ISO: on the node, open the `local` storage → **ISO Images → Download from URL**. Use the latest `ubuntu-24.04.x-live-server-amd64.iso` from `releases.ubuntu.com/24.04/`, and paste its SHA-256 from `SHA256SUMS` into the checksum field.

Then go through **Create VM**, one tab at a time:

| Tab | Settings |
|---|---|
| General | Name `momoshtrem`. Under Advanced, tick **Start at boot**. |
| OS | The ISO. Guest OS: Linux, 6.x – 2.6 Kernel. |
| System | Machine `q35`. BIOS `OVMF (UEFI)`. Tick **Add EFI Disk** (same storage) and **Pre-Enroll keys**. SCSI Controller `VirtIO SCSI single`. Tick **Qemu Agent**. No TPM. |
| Disks | `scsi0`, 32 GiB. Tick Discard and IO thread. Under Advanced, tick SSD emulation (on SSD storage). Add `scsi1`, 120 GiB, with the same options. |
| CPU | 1 socket, 4 cores, type `host`. |
| Memory | 8192 MiB, or what §2 allows. |
| Network | Bridge `vmbr0`, model VirtIO (paravirtualized). |

After creating it, turn on guest-trim: **Options → QEMU Guest Agent → Run guest-trim after a disk move or VM migration**.

The same from the host shell. Replace `<vmid>`, `<storage>` and `<iso>`, and drop `ssd=1` on HDD storage:

```bash
qm create <vmid> --name momoshtrem --ostype l26 --onboot 1 \
  --machine q35 --bios ovmf \
  --efidisk0 <storage>:1,efitype=4m,pre-enrolled-keys=1 \
  --cpu host --sockets 1 --cores 4 --memory 8192 \
  --scsihw virtio-scsi-single \
  --scsi0 <storage>:32,discard=on,iothread=1,ssd=1 \
  --scsi1 <storage>:120,discard=on,iothread=1,ssd=1 \
  --net0 virtio,bridge=vmbr0 \
  --agent enabled=1,fstrim_cloned_disks=1 \
  --ide2 local:iso/<iso>,media=cdrom \
  --boot order='scsi0;ide2'
```

Note the MAC from **Hardware → Network Device**. The cutover needs it.

## 4. Install Ubuntu

Start the VM and open the console. In the installer:

1. **Type of installation:** Ubuntu Server, not minimized. Leave "Search for third-party drivers" off.
2. **Network:** leave DHCP on. Note the address it gets; §6 makes it permanent.
3. **Guided storage:** pick **Use an entire disk** and choose the **32 GiB** disk. Leave "Set up this disk as an LVM group" ticked, and LUKS encryption off.
   - The disks show up as QEMU hard disks. Tell them apart by size, not by name.
4. **Storage configuration** (the summary screen):
   - **Root.** Under Used devices, select `ubuntu-lv` → **Edit**, and set its size to the maximum shown. The default is about half the volume group, which is the old VM's mistake.
   - **Data.** Under Available devices, select the **120 GiB** disk → **Add GPT Partition**. Leave the size empty (the whole disk), format it ext4, and set the mount to **Other** → `/mnt/data`.
   - Check the summary. `/` should be on `ubuntu-lv` and fill the volume group, and `/mnt/data` should be on the 120 GiB disk.
5. **Profile:** server name `momoshtrem`, username `agha`, and a strong password. Save it in the password manager: it's the sudo password.
6. **Ubuntu Pro:** skip.
7. **SSH:** tick **Install OpenSSH server**. Allow password authentication, but only for the first login; §5 turns it off. Don't import keys.
8. **Featured snaps:** select none.

When the installer asks you to remove the installation medium, set **Hardware → CD/DVD Drive → Do not use any media**, then press Enter.

If the root LV was missed in step 4, fix it after the first boot:

```bash
sudo lvextend -r -l +100%FREE ubuntu-vg/ubuntu-lv
```

## 5. First login and the hand-off state

**On the Mac**, add an SSH alias for the VM's IP in `~/.ssh/config`, then copy the key across:

```
Host momoshtrem-v2
  HostName <vm-ip>
  User agha
```

```bash
ssh-copy-id momoshtrem-v2
```

The VM keeps this address, so the alias stays as it is after the cutover (§6).

**On the VM**, make SSH key-only. Keep a second session open until you have checked that you can log in:

```bash
sudo tee /etc/ssh/sshd_config.d/10-key-only.conf >/dev/null <<'EOF'
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin no
EOF
sudo sshd -t && sudo systemctl restart ssh
```

The file is named `10-…` so that it wins over the `50-cloud-init.conf` the installer writes: sshd keeps the first value it reads.

**Hand-off checks.** The playbook (#34) starts from this state. Its `handoff` step checks it and stops the run if anything is off, but it never partitions or formats a disk.

```bash
# On the VM
lsb_release -d                     # Ubuntu 24.04.x LTS
hostnamectl hostname               # momoshtrem
sudo vgs                           # ubuntu-vg: VFree is 0
df -h / /mnt/data                  # / is about 28 GiB, /mnt/data about 118 GiB
findmnt -no SOURCE,FSTYPE /mnt/data && grep /mnt/data /etc/fstab   # ext4, by UUID
lsblk --discard                    # DISC-GRAN and DISC-MAX are non-zero on both disks
systemctl is-enabled fstrim.timer  # enabled
sudo sshd -T | grep -E '^(passwordauthentication|kbdinteractiveauthentication|permitrootlogin) '   # all "no"
snap list                          # no docker
id agha                            # in sudo

# On the Mac: password login must be refused
ssh -o PubkeyAuthentication=no -o PreferredAuthentications=password momoshtrem-v2 true
# Permission denied (publickey).
```

Nothing else is installed or configured by hand. The rest belongs to:

| What | Where |
|---|---|
| Packages, unattended upgrades, Docker and its log caps, journald cap, the guest agent package, ufw, the data folders, the boot unit | #34, [`HOST.md`](HOST.md) |
| Backups, the shared lock and the restore runbook | #35 |
| The deploy agent and heartbeat | #38 |
| `.env`, copying the data, the cutover | #42 |

## 6. Later changes to the VM

These happen at or after the cutover (#42 §3–4). They're listed here because they change the VM or its neighbours.

- **The IP.** The VM keeps 192.168.0.99. On the router, make it a DHCP reservation for this VM's MAC. Once the old VM is shut down, point the port forwards (36911 and 3000) at .99. The old VM keeps its .81 reservation, so a rollback only moves the forwards back (PLAN §6).
- **The SSH alias.** `momoshtrem-v2` doesn't change. Drop `momoshtrem-vm` once the old VM is deleted.
- **RAM.** Raise it to 8 GiB once the old VM is off: shut the VM down, set 8192 MiB and start it again. It was built with about 6 GiB for the overlap (§2).
- **The old VM.** Turn its **Start at boot** off once it's stopped. Otherwise a host reboot brings it back, with its own stack, until it's deleted.
- **The data disk.** After the cutover, grow it from 70 to 120 GiB:
  1. In Proxmox, **Hardware → Hard Disk (scsi1) → Disk Action → Resize**, and add 50 GiB.
  2. On the VM, check with `lsblk` that the data disk is still `sdb`, then grow the partition and the filesystem while mounted:
     ```bash
     sudo growpart /dev/sdb 1 && sudo resize2fs /dev/sdb1
     ```
  3. Raise `global_cache_size` to 92160 in `config.yaml` and restart momoshtrem.
