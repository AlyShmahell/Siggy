# Shared arrow/enter/esc start menu for Siggy run scripts.
# Source this file; do not execute it.

menu_c_reset=$'\033[0m'
menu_c_bold=$'\033[1m'
menu_c_dim=$'\033[2m'
menu_c_cyan=$'\033[36m'
menu_c_mag=$'\033[35m'
menu_c_yel=$'\033[33m'
menu_c_red=$'\033[31m'
menu_c_inv=$'\033[7m'

menu_hide() { printf '\033[?25l' >&2; }
menu_show() { printf '\033[?25h' >&2; }

menu_cleanup() {
  menu_show
  stty echo icanon 2>/dev/null || true
}

menu_draw() {
  local title="$1"
  local selected="$2"
  shift 2
  local items=("$@")
  local i
  printf '\033[2J\033[H' >&2
  printf '%s%s siggy %s%s\n' "$menu_c_bold" "$menu_c_mag" "$menu_c_reset" "$menu_c_dim" >&2
  printf '%s\n\n' "$title" >&2
  for i in "${!items[@]}"; do
    if [[ "$i" -eq "$selected" ]]; then
      printf '  %s%s ▸ %s %s\n' "$menu_c_inv" "$menu_c_cyan" "${items[$i]}" "$menu_c_reset" >&2
    else
      printf '    %s%s%s\n' "$menu_c_dim" "${items[$i]}" "$menu_c_reset" >&2
    fi
  done
  printf '\n%s↑/↓ move   enter select   esc quit%s\n' "$menu_c_yel" "$menu_c_reset" >&2
}

# menu_select TITLE ITEM [ITEM...]
# prints the chosen item name to stdout; returns 1 on cancel.
menu_select() {
  local title="$1"
  shift
  local items=("$@")
  local n="${#items[@]}"
  local idx=0
  local key

  if [[ ! -t 0 || ! -t 2 ]]; then
    printf '%serror:%s interactive menu needs a tty (use: ./run -- <item>)\n' "$menu_c_red" "$menu_c_reset" >&2
    return 1
  fi

  trap menu_cleanup EXIT INT TERM
  stty -echo -icanon
  menu_hide
  menu_draw "$title" "$idx" "${items[@]}"

  while true; do
    if ! IFS= read -rsn1 key; then
      menu_cleanup
      trap - EXIT INT TERM
      return 1
    fi
    case "$key" in
      $'\033')
        IFS= read -rsn1 -t 0.05 key || true
        if [[ -z "${key:-}" ]]; then
          menu_cleanup
          trap - EXIT INT TERM
          return 1
        fi
        if [[ "$key" == "[" ]]; then
          IFS= read -rsn1 -t 0.05 key || true
          case "$key" in
            A) idx=$(( (idx - 1 + n) % n )) ;;
            B) idx=$(( (idx + 1) % n )) ;;
          esac
        fi
        ;;
      "")
        menu_cleanup
        trap - EXIT INT TERM
        printf '%s\n' "${items[$idx]}"
        return 0
        ;;
      q|Q)
        menu_cleanup
        trap - EXIT INT TERM
        return 1
        ;;
    esac
    menu_draw "$title" "$idx" "${items[@]}"
  done
}

# menu_run TITLE ITEM:CMD ITEM:CMD ...
# If argv contains `--`, treat the next arg as the item name (one-shot).
# Remaining args after the item name are forwarded via MENU_ARGS.
# Interactive: run the chosen command, then return to the menu. Esc/q quits.
# Command failure must not kill the loop (callers may use set -e).
menu_run() {
  local title="$1"
  shift
  local pairs=("$@")
  local names=()
  local cmds=()
  local pair name cmd chosen i found

  for pair in "${pairs[@]}"; do
    name="${pair%%:*}"
    cmd="${pair#*:}"
    names+=("$name")
    cmds+=("$cmd")
  done

  MENU_ARGS=()
  if [[ -z "${MENU_CLI_ITEM+x}" ]]; then
    MENU_CLI_ITEM=()
  fi
  if [[ ${#MENU_CLI_ITEM[@]} -gt 0 ]]; then
    chosen="${MENU_CLI_ITEM[0]}"
    MENU_ARGS=("${MENU_CLI_ITEM[@]:1}")
    for i in "${!names[@]}"; do
      if [[ "${names[$i]}" == "$chosen" ]]; then
        # shellcheck disable=SC2086
        eval "${cmds[$i]}"
        return $?
      fi
    done
    printf '%sunknown item:%s %s\n' "$menu_c_red" "$menu_c_reset" "$chosen" >&2
    printf 'available:' >&2
    printf ' %s' "${names[@]}" >&2
    printf '\n' >&2
    return 1
  fi

  while true; do
    chosen="$(menu_select "$title" "${names[@]}")" || {
      printf '%scancelled%s\n' "$menu_c_dim" "$menu_c_reset" >&2
      return 1
    }
    found=0
    for i in "${!names[@]}"; do
      if [[ "${names[$i]}" == "$chosen" ]]; then
        found=1
        # shellcheck disable=SC2086
        eval "${cmds[$i]}" || true
        break
      fi
    done
    if [[ "$found" -eq 0 ]]; then
      printf '%sunknown item:%s %s\n' "$menu_c_red" "$menu_c_reset" "$chosen" >&2
      printf 'available:' >&2
      printf ' %s' "${names[@]}" >&2
      printf '\n' >&2
      return 1
    fi
  done
}

menu_parse_cli() {
  MENU_CLI_ITEM=()
  if [[ "${1:-}" == "--" ]]; then
    shift
    MENU_CLI_ITEM=("$@")
  elif [[ $# -gt 0 ]]; then
    MENU_CLI_ITEM=("$@")
  fi
}
