// everest-operator
// Copyright (C) 2022 Percona LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

const (
	// pxcConfigSizeSmall is the configuration for PXC cluster with the dimension of 1 vCPU and 2GB RAM
	pxcConfigSizeSmall = `[mysqld]
innodb_adaptive_hash_index = True
innodb_log_files_in_group = 2
innodb_parallel_read_threads = 1
innodb_buffer_pool_instances = 1
innodb_flush_method = O_DIRECT
innodb_log_file_size = 152548048
innodb_page_cleaners = 1
innodb_purge_threads = 4
innodb_buffer_pool_chunk_size = 2097152
innodb_buffer_pool_size = 731424661
innodb_ddl_threads = 2
innodb_flush_log_at_trx_commit = 2
innodb_io_capacity_max = 1800
innodb_monitor_enable = ALL
replica_parallel_workers = 4
replica_preserve_commit_order = ON
replica_compressed_protocol = 1
replica_exec_mode = STRICT
replica_parallel_type = LOGICAL_CLOCK
loose_group_replication_member_expel_timeout = 6
loose_group_replication_autorejoin_tries = 3
loose_group_replication_message_cache_size = 162538812
loose_group_replication_communication_max_message_size = 10485760
loose_group_replication_unreachable_majority_timeout = 1029
loose_group_replication_poll_spin_loops = 20000
loose_group_replication_paxos_single_leader = ON
loose_binlog_transaction_dependency_tracking = WRITESET
read_rnd_buffer_size = 393216
sort_buffer_size = 524288
max_heap_table_size = 16777216
tmp_table_size = 16777216
binlog_cache_size = 131072
binlog_stmt_cache_size = 131072
join_buffer_size = 524288
max_connections = 72
tablespace_definition_cache = 512
thread_cache_size = 9
thread_stack = 1024
table_open_cache_instances = 4
sync_binlog = 1
sql_mode = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION,TRADITIONAL,STRICT_ALL_TABLES'
binlog_expire_logs_seconds = 604800
thread_pool_size = 4
table_definition_cache = 4096
table_open_cache = 4096
binlog_format = ROW
    `
	// pxcConfigSizeMedium is the configuration for PXC cluster with the dimension of 4 vCPU and 8GB RAM
	pxcConfigSizeMedium = `[mysqld]
replica_parallel_type = LOGICAL_CLOCK
replica_parallel_workers = 4
replica_preserve_commit_order = ON
replica_compressed_protocol = 1
replica_exec_mode = STRICT
loose_group_replication_message_cache_size = 680057301
loose_group_replication_communication_max_message_size = 10485760
loose_group_replication_unreachable_majority_timeout = 1029
loose_group_replication_poll_spin_loops = 20000
loose_group_replication_paxos_single_leader = ON
loose_binlog_transaction_dependency_tracking = WRITESET
loose_group_replication_member_expel_timeout = 6
loose_group_replication_autorejoin_tries = 3
sort_buffer_size = 524288
max_heap_table_size = 16777216
tmp_table_size = 16777216
binlog_cache_size = 131072
binlog_stmt_cache_size = 131072
join_buffer_size = 524288
read_rnd_buffer_size = 393216
table_definition_cache = 4096
table_open_cache_instances = 4
binlog_expire_logs_seconds = 604800
binlog_format = ROW
max_connections = 72
thread_pool_size = 6
table_open_cache = 4096
thread_stack = 1024
tablespace_definition_cache = 512
sync_binlog = 1
sql_mode = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION,TRADITIONAL,STRICT_ALL_TABLES'
thread_cache_size = 9
innodb_buffer_pool_size = 3060257859
innodb_ddl_threads = 2
innodb_flush_log_at_trx_commit = 2
innodb_log_files_in_group = 5
innodb_purge_threads = 4
innodb_buffer_pool_chunk_size = 2097152
innodb_adaptive_hash_index = True
innodb_flush_method = O_DIRECT
innodb_log_file_size = 251255603
innodb_page_cleaners = 3
innodb_buffer_pool_instances = 3
innodb_io_capacity_max = 1800
innodb_parallel_read_threads = 3
innodb_monitor_enable = ALL
	`
	// pxcConfigSizeLarge is the configuration for PXC cluster with the dimension of 8 vCPU and 32GB RAM
	pxcConfigSizeLarge = `[mysqld]
replica_compressed_protocol = 1
replica_exec_mode = STRICT
replica_parallel_type = LOGICAL_CLOCK
replica_parallel_workers = 4
replica_preserve_commit_order = ON
loose_group_replication_member_expel_timeout = 6
loose_group_replication_autorejoin_tries = 3
loose_group_replication_message_cache_size = 2729928056
loose_group_replication_communication_max_message_size = 10485760
loose_group_replication_unreachable_majority_timeout = 1022
loose_group_replication_poll_spin_loops = 20000
loose_group_replication_paxos_single_leader = ON
loose_binlog_transaction_dependency_tracking = WRITESET
sort_buffer_size = 524288
max_heap_table_size = 16777216
tmp_table_size = 16777216
binlog_cache_size = 131072
binlog_stmt_cache_size = 131072
join_buffer_size = 524288
read_rnd_buffer_size = 393216
table_open_cache_instances = 4
tablespace_definition_cache = 512
sync_binlog = 1
sql_mode = 'ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION,TRADITIONAL,STRICT_ALL_TABLES'
binlog_format = ROW
thread_cache_size = 9
table_definition_cache = 4096
thread_pool_size = 12
table_open_cache = 4096
thread_stack = 1024
binlog_expire_logs_seconds = 604800
max_connections = 72
innodb_adaptive_hash_index = True
innodb_log_files_in_group = 11
innodb_buffer_pool_size = 12284676255
innodb_buffer_pool_instances = 13
innodb_flush_log_at_trx_commit = 2
innodb_purge_threads = 8
innodb_parallel_read_threads = 6
innodb_monitor_enable = ALL
innodb_ddl_threads = 2
innodb_flush_method = O_DIRECT
innodb_page_cleaners = 13
innodb_log_file_size = 456113989
innodb_io_capacity_max = 1800
innodb_buffer_pool_chunk_size = 2097152
	`
)
