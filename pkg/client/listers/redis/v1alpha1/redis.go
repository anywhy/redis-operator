/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/anywhy/redis-operator/pkg/apis/redis/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// RedisLister helps list Redises.
type RedisLister interface {
	// List lists all Redises in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.Redis, err error)
	// Redises returns an object that can list and get Redises.
	Redises(namespace string) RedisNamespaceLister
	RedisListerExpansion
}

// redisLister implements the RedisLister interface.
type redisLister struct {
	indexer cache.Indexer
}

// NewRedisLister returns a new RedisLister.
func NewRedisLister(indexer cache.Indexer) RedisLister {
	return &redisLister{indexer: indexer}
}

// List lists all Redises in the indexer.
func (s *redisLister) List(selector labels.Selector) (ret []*v1alpha1.Redis, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.Redis))
	})
	return ret, err
}

// Redises returns an object that can list and get Redises.
func (s *redisLister) Redises(namespace string) RedisNamespaceLister {
	return redisNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// RedisNamespaceLister helps list and get Redises.
type RedisNamespaceLister interface {
	// List lists all Redises in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1alpha1.Redis, err error)
	// Get retrieves the Redis from the indexer for a given namespace and name.
	Get(name string) (*v1alpha1.Redis, error)
	RedisNamespaceListerExpansion
}

// redisNamespaceLister implements the RedisNamespaceLister
// interface.
type redisNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all Redises in the indexer for a given namespace.
func (s redisNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.Redis, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.Redis))
	})
	return ret, err
}

// Get retrieves the Redis from the indexer for a given namespace and name.
func (s redisNamespaceLister) Get(name string) (*v1alpha1.Redis, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("redis"), name)
	}
	return obj.(*v1alpha1.Redis), nil
}