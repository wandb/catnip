" Basic display settings
set termguicolors
colorscheme zaibatsu
syntax enable
set number
set relativenumber
set cursorline
set showmatch
set ruler
set laststatus=2

" Indentation and tab settings
set autoindent
set smartindent
set expandtab
set tabstop=4
set shiftwidth=4
set softtabstop=4
set smarttab

" Search settings
set incsearch
set hlsearch
set ignorecase
set smartcase

" Better backspace behavior
set backspace=indent,eol,start

" Mouse support
set mouse=a

" File handling
set encoding=utf-8
set fileencoding=utf-8
set autoread
set noswapfile
set nobackup
set nowritebackup

" Performance improvements
set lazyredraw
set ttyfast

" Better command completion
set wildmenu
set wildmode=longest:full,full

" Show current mode
set showmode
set showcmd

" Better splitting
set splitbelow
set splitright

" Folding
set foldmethod=indent
set foldlevelstart=99

" Better scrolling
set scrolloff=8
set sidescrolloff=8

" Clipboard integration (if available)
if has("clipboard")
    set clipboard=unnamedplus
endif

" Key mappings
" Clear search highlighting with Esc
nnoremap <silent> <Esc> :nohlsearch<CR><Esc>

" Better window navigation
nnoremap <C-h> <C-w>h
nnoremap <C-j> <C-w>j
nnoremap <C-k> <C-w>k
nnoremap <C-l> <C-w>l

" Quick save
nnoremap <C-s> :w<CR>
inoremap <C-s> <Esc>:w<CR>a

" File type specific settings
autocmd FileType python setlocal tabstop=4 shiftwidth=4 softtabstop=4
autocmd FileType javascript,typescript,json,html,css setlocal tabstop=2 shiftwidth=2 softtabstop=2
autocmd FileType go setlocal tabstop=4 shiftwidth=4 softtabstop=4 noexpandtab
autocmd FileType yaml setlocal tabstop=2 shiftwidth=2 softtabstop=2
autocmd FileType markdown setlocal wrap linebreak

" Remove trailing whitespace on save
autocmd BufWritePre * :%s/\s\+$//e